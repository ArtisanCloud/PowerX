package capability_registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	"github.com/redis/go-redis/v9"
)

// capabilityAuthorizer defines the contract Selector expects.
type capabilityAuthorizer interface {
	AuthorizeInvocation(ctx context.Context, req CapabilityInvokeRequest) error
}

// AuthorizationOptions wires AuthorizationService dependencies.
type AuthorizationOptions struct {
	Catalog      CapabilityLookup
	SafeMode     SafeModeStore
	Environment  string
	FeatureFlags FeatureFlagProvider
}

// AuthorizationService validates tenant state (safe-mode, feature flags, tool grants).
type AuthorizationService struct {
	catalog      CapabilityLookup
	safeMode     SafeModeStore
	featureFlags FeatureFlagProvider
}

// CapabilityLookup fetches capability metadata.
type CapabilityLookup interface {
	GetCapability(ctx context.Context, capabilityID string, includeWorkflows bool) (CapabilityRecordView, error)
}

// SafeModeStore exposes tenant safe-mode toggle state.
type SafeModeStore interface {
	State(ctx context.Context, tenantUUID string) (SafeModeState, error)
}

// FeatureFlagProvider exposes per-tenant feature toggles.
type FeatureFlagProvider interface {
	Allowed(ctx context.Context, tenantUUID string, flags []string) (bool, error)
}

// SafeModeState mirrors agent model hub toggle info.
type SafeModeState struct {
	TenantUUID string
	Enabled    bool
	Reason     string
	Actor      string
	ExpiresAt  *time.Time
	UpdatedAt  time.Time
}

// NewAuthorizationService builds an authorization helper.
func NewAuthorizationService(opts AuthorizationOptions) *AuthorizationService {
	return &AuthorizationService{
		catalog:      opts.Catalog,
		safeMode:     opts.SafeMode,
		featureFlags: opts.FeatureFlags,
	}
}

// AuthorizeInvocation enforces safe-mode, feature flag, and tool grant policies.
func (s *AuthorizationService) AuthorizeInvocation(ctx context.Context, req CapabilityInvokeRequest) error {
	if s == nil {
		return nil
	}
	tenant := strings.TrimSpace(req.TenantUUID)
	if tenant == "" {
		return ErrSelectorTenantRequired
	}
	if s.safeMode != nil {
		state, err := s.safeMode.State(ctx, tenant)
		if err != nil {
			return err
		}
		if state.Enabled && !hasSafeModeBypass(req.Context) {
			return ErrSelectorSafeModeActive
		}
	}

	if s.featureFlags != nil {
		flags := extractFeatureFlags(req.Context)
		if allowed, err := s.featureFlags.Allowed(ctx, tenant, flags); err != nil {
			return err
		} else if !allowed {
			return ErrSelectorFeatureFlagMissing
		}
	}

	if s.catalog == nil || strings.TrimSpace(req.CapabilityID) == "" {
		return nil
	}
	view, err := s.catalog.GetCapability(ctx, strings.TrimSpace(req.CapabilityID), false)
	if err != nil {
		return err
	}
	if view.Record == nil {
		return ErrSelectorCapabilityRequired
	}
	policy := parseCapabilityPolicy(view.Record.Policy)
	if requiresToolGrants(policy) {
		if len(req.ToolGrantIDs) == 0 {
			return ErrSelectorToolGrantRequired
		}
		if !policy.AllowsToolGrants(req.ToolGrantIDs) {
			return ErrSelectorCapabilityForbidden
		}
	}

	if requiredFlags := capabilityRequiresFeatureFlags(view.Record); len(requiredFlags) > 0 {
		if provided := extractFeatureFlags(req.Context); len(provided) > 0 {
			if !containsAllFlags(provided, requiredFlags) {
				return ErrSelectorFeatureFlagMissing
			}
		} else if s.featureFlags != nil {
			allowed, err := s.featureFlags.Allowed(ctx, tenant, requiredFlags)
			if err != nil {
				return err
			}
			if !allowed {
				return ErrSelectorFeatureFlagMissing
			}
		} else {
			return ErrSelectorFeatureFlagMissing
		}
	}
	return nil
}

func requiresToolGrants(policy capabilityPolicy) bool {
	rule := policy.Visibility.toolGrantRule()
	return len(rule.Allow) > 0
}

func hasSafeModeBypass(ctx map[string]interface{}) bool {
	if len(ctx) == 0 {
		return false
	}
	if raw, ok := ctx["safe_mode_bypass"]; ok {
		if flag, ok := raw.(bool); ok {
			return flag
		}
		if str, ok := raw.(string); ok {
			return strings.EqualFold(str, "true")
		}
	}
	return false
}

func extractFeatureFlags(ctx map[string]interface{}) []string {
	if len(ctx) == 0 {
		return nil
	}
	raw, ok := ctx["feature_flags"]
	if !ok {
		return nil
	}
	switch typed := raw.(type) {
	case []string:
		return append([]string{}, typed...)
	case []interface{}:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if str, ok := item.(string); ok && strings.TrimSpace(str) != "" {
				result = append(result, strings.TrimSpace(str))
			}
		}
		return result
	case string:
		items := strings.Split(typed, ",")
		result := make([]string, 0, len(items))
		for _, item := range items {
			item = strings.TrimSpace(item)
			if item != "" {
				result = append(result, item)
			}
		}
		return result
	default:
		return nil
	}
}

// RedisSafeModeStore reads safe-mode state from redis.
type RedisSafeModeStore struct {
	client      redis.UniversalClient
	keyPrefix   string
	environment string
}

// SafeModeStoreOptions configures the redis safe-mode reader.
type SafeModeStoreOptions struct {
	Redis       redis.UniversalClient
	KeyPrefix   string
	Environment string
}

// NewRedisSafeModeStore builds a store backed by redis.
func NewRedisSafeModeStore(opts SafeModeStoreOptions) *RedisSafeModeStore {
	if opts.Redis == nil {
		return nil
	}
	prefix := strings.TrimSpace(opts.KeyPrefix)
	if prefix == "" {
		prefix = "agent:modelhub:safe_mode"
	}
	env := strings.TrimSpace(opts.Environment)
	if env == "" {
		env = "default"
	}
	return &RedisSafeModeStore{
		client:      opts.Redis,
		keyPrefix:   prefix,
		environment: env,
	}
}

// State implements SafeModeStore.
func (s *RedisSafeModeStore) State(ctx context.Context, tenantUUID string) (SafeModeState, error) {
	if s == nil || s.client == nil {
		return SafeModeState{TenantUUID: tenantUUID}, nil
	}
	tenant := strings.TrimSpace(tenantUUID)
	if tenant == "" {
		return SafeModeState{}, errors.New("tenant uuid required")
	}
	key := fmt.Sprintf("%s:%s:%s", s.keyPrefix, s.environment, tenant)
	raw, err := s.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return SafeModeState{TenantUUID: tenant, Enabled: false}, nil
	}
	if err != nil {
		return SafeModeState{}, err
	}
	var record safeModeRecordSnapshot
	if err := json.Unmarshal(raw, &record); err != nil {
		return SafeModeState{}, err
	}
	state := SafeModeState{
		TenantUUID: tenant,
		Enabled:    record.Enabled,
		Reason:     record.Reason,
		Actor:      record.Actor,
	}
	if record.UpdatedAt != "" {
		if ts, err := time.Parse(time.RFC3339, record.UpdatedAt); err == nil {
			state.UpdatedAt = ts
		}
	}
	if record.ExpiresAt != "" {
		if ts, err := time.Parse(time.RFC3339, record.ExpiresAt); err == nil {
			state.ExpiresAt = &ts
		}
	}
	return state, nil
}

type safeModeRecordSnapshot struct {
	Enabled   bool   `json:"enabled"`
	Reason    string `json:"reason"`
	Actor     string `json:"actor"`
	UpdatedAt string `json:"updated_at"`
	ExpiresAt string `json:"expires_at"`
}

// DefaultFeatureFlagProvider allows every request. Placeholder for future integrations.
type DefaultFeatureFlagProvider struct{}

// Allowed always returns true.
func (DefaultFeatureFlagProvider) Allowed(context.Context, string, []string) (bool, error) {
	return true, nil
}

// capabilityRequiresFeatureFlags extracts required flags from annotations.
func capabilityRequiresFeatureFlags(record *models.CapabilityRecord) []string {
	if record == nil || len(record.Annotations) == 0 {
		return nil
	}
	var annotations map[string]interface{}
	if err := json.Unmarshal(record.Annotations, &annotations); err != nil {
		return nil
	}
	raw, ok := annotations["feature_flags"]
	if !ok {
		return nil
	}
	switch typed := raw.(type) {
	case []interface{}:
		flags := make([]string, 0, len(typed))
		for _, item := range typed {
			if str, ok := item.(string); ok && strings.TrimSpace(str) != "" {
				flags = append(flags, strings.TrimSpace(str))
			}
		}
		return flags
	case []string:
		return append([]string{}, typed...)
	default:
		return nil
	}
}

func containsAllFlags(have, required []string) bool {
	if len(required) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(have))
	for _, flag := range have {
		flag = strings.TrimSpace(flag)
		if flag == "" {
			continue
		}
		set[strings.ToLower(flag)] = struct{}{}
	}
	for _, flag := range required {
		flag = strings.TrimSpace(flag)
		if flag == "" {
			continue
		}
		if _, ok := set[strings.ToLower(flag)]; !ok {
			return false
		}
	}
	return true
}
