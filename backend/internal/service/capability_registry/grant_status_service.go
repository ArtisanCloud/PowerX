package capability_registry

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	iammodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	settingmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	gwrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	settingrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/gorm"
)

const (
	GrantStatusCapabilityID = "com.corex.capabilities.grant_status.read"

	GrantStatusGranted    = "granted"
	GrantStatusNotGranted = "not_granted"
	GrantStatusUnknown    = "unknown"

	GrantStatusReasonGranted             = "CAPABILITY_GRANTED"
	GrantStatusReasonNotGranted          = "CAPABILITY_NOT_GRANTED"
	GrantStatusReasonUnknown             = "CAPABILITY_UNKNOWN"
	GrantStatusReasonTenantNotRegistered = "CAPABILITY_TENANT_NOT_REGISTERED"
)

var (
	ErrGrantStatusUnauthorized = errors.New("capability.grant_status_unauthorized")
	ErrGrantStatusForbidden    = errors.New("capability.grant_status_forbidden")
	ErrGrantStatusInvalid      = errors.New("capability.grant_status_invalid_argument")
	ErrGrantStatusUnavailable  = errors.New("capability.grant_status_upstream_dependency")

	capabilityIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)
)

// GrantStatusItem retains the request order and never reveals another actor's
// tenant, plugin identity, API-key profile, or underlying permission rows.
type GrantStatusItem struct {
	CapabilityID string `json:"capability_id"`
	Status       string `json:"status"`
	ReasonCode   string `json:"reason_code"`
}

// GrantStatusService evaluates the current credential only. It deliberately
// does not use sts_direct as an authorization signal.
type GrantStatusService struct{ db *gorm.DB }

func NewGrantStatusService(db *gorm.DB) *GrantStatusService { return &GrantStatusService{db: db} }

func (s *GrantStatusService) CheckCurrentCredential(ctx context.Context, capabilityIDs []string) ([]GrantStatusItem, error) {
	if s == nil || s.db == nil {
		return nil, ErrGrantStatusUnavailable
	}
	if err := validateGrantStatusCapabilityIDs(capabilityIDs); err != nil {
		return nil, err
	}
	tenantUUID, err := reqctx.RequireTenantUUID(ctx)
	if err != nil {
		return nil, ErrGrantStatusUnauthorized
	}
	tenantUUID, err = reqctx.CanonicalTenantUUID(tenantUUID)
	if err != nil {
		return nil, ErrGrantStatusUnauthorized
	}
	claims := reqctx.GetClaims(ctx)
	if claims == nil {
		return nil, ErrGrantStatusUnauthorized
	}
	var allowed map[string]bool
	apiKeyScopeGranted := false
	if grantStatusContains(claims.Platforms, "api_key") {
		allowed, apiKeyScopeGranted, err = s.allowedForGatewayAPIKey(ctx, tenantUUID)
	} else {
		if !strings.EqualFold(strings.TrimSpace(claims.Issuer), "powerx-sts") || !grantStatusContains(claims.Audience, "powerx:api") || strings.TrimSpace(claims.PluginID) == "" {
			return nil, ErrGrantStatusForbidden
		}
		allowed, err = s.allowedForSTSPlugin(ctx, tenantUUID, strings.TrimSpace(claims.PluginID))
	}
	if err != nil {
		return nil, err
	}
	if err := s.requireGrantStatusCapability(ctx, tenantUUID, allowed, !grantStatusContains(claims.Platforms, "api_key") || apiKeyScopeGranted); err != nil {
		return nil, err
	}
	published, err := s.publishedAndRegistered(ctx, tenantUUID, capabilityIDs)
	if err != nil {
		return nil, err
	}
	return buildGrantStatusItems(capabilityIDs, published, allowed), nil
}

type publishedGrantStatus struct {
	published  map[string]bool
	registered map[string]bool
}

func (s *GrantStatusService) publishedAndRegistered(ctx context.Context, tenantUUID string, capabilityIDs []string) (publishedGrantStatus, error) {
	result := publishedGrantStatus{published: make(map[string]bool, len(capabilityIDs)), registered: make(map[string]bool, len(capabilityIDs))}
	var capabilities []capmodels.CapabilityRecord
	if err := s.db.WithContext(ctx).Where("capability_id IN ? AND status = ?", capabilityIDs, "published").Find(&capabilities).Error; err != nil {
		return result, ErrGrantStatusUnavailable
	}
	for _, capability := range capabilities {
		result.published[capability.CapabilityID] = true
	}
	if len(capabilities) == 0 {
		return result, nil
	}
	var registrations []capmodels.CapabilityRegistration
	if err := s.db.WithContext(ctx).Where("tenant_uuid = ? AND capability_id IN ?", tenantUUID, capabilityIDs).Order("version DESC").Find(&registrations).Error; err != nil {
		return result, ErrGrantStatusUnavailable
	}
	seen := map[string]bool{}
	for _, registration := range registrations {
		if seen[registration.CapabilityID] {
			continue
		}
		seen[registration.CapabilityID] = true
		result.registered[registration.CapabilityID] = registration.Status == "published"
	}
	return result, nil
}

func (s *GrantStatusService) allowedForSTSPlugin(ctx context.Context, tenantUUID, pluginID string) (map[string]bool, error) {
	var credential settingmodel.PluginInstanceConfig
	err := s.db.WithContext(ctx).Where("tenant_uuid = ? AND plugin_id = ? AND key = ? AND enabled = ?", tenantUUID, pluginID, settingrepo.KeyClientCredentials, true).First(&credential).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrGrantStatusUnavailable
	}
	allowed := map[string]bool{}
	if err == nil {
		var payload struct {
			AllowedCapabilities []string `json:"allowed_capabilities"`
		}
		if json.Unmarshal(credential.ValueJSON, &payload) != nil {
			return nil, ErrGrantStatusUnavailable
		}
		for _, capabilityID := range payload.AllowedCapabilities {
			allowed[strings.TrimSpace(capabilityID)] = true
		}
	}
	return allowed, nil
}

func (s *GrantStatusService) allowedForGatewayAPIKey(ctx context.Context, tenantUUID string) (map[string]bool, bool, error) {
	hash := reqctx.AuthenticatedAPIKeyHash(ctx)
	if hash == "" {
		return nil, false, ErrGrantStatusUnauthorized
	}
	key, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(s.db).FindActiveByHash(ctx, tenantUUID, hash)
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && key == nil) {
		return nil, false, ErrGrantStatusUnauthorized
	}
	if err != nil {
		return nil, false, ErrGrantStatusUnavailable
	}
	var definitions []iammodel.Permission
	if err := s.db.WithContext(ctx).Where("status = ? AND effect = ? AND allow_api_key = ?", iammodel.PermissionStatusActive, "allow", true).Find(&definitions).Error; err != nil {
		return nil, false, ErrGrantStatusUnavailable
	}
	allowed := map[string]bool{}
	permissions, err := gwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(s.db).ListByAPIKeyUUID(ctx, key.UUID)
	if err != nil {
		return nil, false, ErrGrantStatusUnavailable
	}
	for _, definition := range definitions {
		var meta struct {
			CapabilityID string `json:"capability_id"`
			Explicit     bool   `json:"api_key_explicit"`
			APIKey       struct {
				Scope        string `json:"scope"`
				Action       string `json:"action"`
				ResourceType string `json:"resource_type"`
				Resource     string `json:"resource_pattern"`
				Effect       string `json:"effect"`
			} `json:"api_key"`
		}
		if json.Unmarshal(definition.Meta, &meta) != nil {
			return nil, false, ErrGrantStatusUnavailable
		}
		if !meta.Explicit || meta.CapabilityID == "" || meta.APIKey.Effect != "allow" {
			continue
		}
		granted := gwrepo.APIKeyPermissionGranted(permissions, meta.APIKey.Scope, meta.APIKey.Action, meta.APIKey.ResourceType, meta.APIKey.Resource)
		if granted {
			allowed[meta.CapabilityID] = true
		}
	}
	return allowed, allowed[GrantStatusCapabilityID], nil
}

func buildGrantStatusItems(capabilityIDs []string, published publishedGrantStatus, allowed map[string]bool) []GrantStatusItem {
	items := make([]GrantStatusItem, 0, len(capabilityIDs))
	for _, capabilityID := range capabilityIDs {
		item := GrantStatusItem{CapabilityID: capabilityID, Status: GrantStatusUnknown, ReasonCode: GrantStatusReasonUnknown}
		if !published.published[capabilityID] {
			items = append(items, item)
			continue
		}
		if !published.registered[capabilityID] {
			item.ReasonCode = GrantStatusReasonTenantNotRegistered
			items = append(items, item)
			continue
		}
		item.Status = GrantStatusNotGranted
		item.ReasonCode = GrantStatusReasonNotGranted
		if allowed[capabilityID] {
			item.Status = GrantStatusGranted
			item.ReasonCode = GrantStatusReasonGranted
		}
		items = append(items, item)
	}
	return items
}

func (s *GrantStatusService) requireGrantStatusCapability(ctx context.Context, tenantUUID string, allowed map[string]bool, apiKeyScopeGranted bool) error {
	state, err := s.publishedAndRegistered(ctx, tenantUUID, []string{GrantStatusCapabilityID})
	if err != nil {
		return err
	}
	if !state.published[GrantStatusCapabilityID] || !state.registered[GrantStatusCapabilityID] || !allowed[GrantStatusCapabilityID] || !apiKeyScopeGranted {
		return ErrGrantStatusForbidden
	}
	return nil
}

func validateGrantStatusCapabilityIDs(capabilityIDs []string) error {
	if len(capabilityIDs) == 0 || len(capabilityIDs) > 100 {
		return ErrGrantStatusInvalid
	}
	seen := make(map[string]struct{}, len(capabilityIDs))
	for _, capabilityID := range capabilityIDs {
		if !capabilityIDPattern.MatchString(capabilityID) {
			return ErrGrantStatusInvalid
		}
		if _, exists := seen[capabilityID]; exists {
			return ErrGrantStatusInvalid
		}
		seen[capabilityID] = struct{}{}
	}
	return nil
}

func grantStatusContains(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}
