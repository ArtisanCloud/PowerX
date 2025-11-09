package provider_registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/instrumentation"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/shared"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	"github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent_model_hub"
	dbmaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/agent_model_hub"
	"github.com/ArtisanCloud/PowerX/pkg/corex/tenantkeys"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	providerHealthCacheKey   = "agent:modelhub:provider:%s:health"
	providerRotationCacheKey = "agent:modelhub:provider:%s:last_rotation"
	defaultRotationInterval  = 12 * time.Hour
)

// Service coordinates provider onboarding, secret management, and rollout governance.
type Service struct {
	db     *gorm.DB
	cache  cache.ICache
	audit  audit.Service
	keys   *tenantkeys.TenantKeyService
	repo   *repo.ProviderProfileRepository
	inst   *instrumentation.Instrumentation
	clock  func() time.Time
	rotMgr *secretRotationScheduler
}

// Options wires ProviderRegistry dependencies.
type Options struct {
	shared.Options
	Profiles *repo.ProviderProfileRepository
}

// NewService constructs a provider registry service with safe defaults.
func NewService(opts Options) *Service {
	opts.Options.Normalize()
	repository := opts.Profiles
	if repository == nil && opts.DB != nil {
		repository = repo.NewProviderProfileRepository(opts.DB)
	}
	svc := &Service{
		db:    opts.DB,
		cache: opts.Cache,
		audit: opts.AuditSvc,
		keys:  opts.TenantKeySvc,
		repo:  repository,
		inst:  opts.Instrumentation,
		clock: opts.Clock,
	}
	svc.rotMgr = newSecretRotationScheduler(svc)
	return svc
}

// ProviderProfileInput captures API payload needed for onboarding.
type ProviderProfileInput struct {
	Name            string
	Capabilities    []string
	PrimaryEndpoint string
	Regions         []string
	TenantWhitelist []TenantRef
	Credentials     map[string]string
	RolloutStatus   string
	AuditTrailID    string
}

// TenantRef mirrors the spec-defined tenant scope contract.
type TenantRef struct {
	TenantID    string `json:"tenantId"`
	Environment string `json:"environment"`
}

// RegisterProvider stores or updates a provider profile using Vault-style sealed secrets.
func (s *Service) RegisterProvider(ctx context.Context, env string, tenantID *uint64, input ProviderProfileInput) (*model.ProviderProfile, error) {
	ctx, _ = instrumentation.EnsureTraceContext(ctx)
	if s.repo == nil {
		return nil, errors.New("provider repository is not configured")
	}
	if s.keys == nil {
		return nil, errors.New("tenant key service is required for Vault sealing")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errors.New("provider name required")
	}
	endpoint := strings.TrimSpace(input.PrimaryEndpoint)
	if endpoint == "" {
		return nil, errors.New("primary endpoint required")
	}

	profile := &model.ProviderProfile{
		Env:             strings.TrimSpace(env),
		TenantID:        tenantID,
		Name:            name,
		PrimaryEndpoint: endpoint,
		RolloutStatus:   statusOrDefault(input.RolloutStatus),
		AuditTrailID:    strings.TrimSpace(input.AuditTrailID),
		Capabilities:    datatypes.JSONSlice[string](append([]string(nil), input.Capabilities...)),
		Regions:         datatypes.JSONSlice[string](append([]string(nil), input.Regions...)),
		HealthScore:     0,
	}
	profile.UUID = uuid.New()
	if len(input.TenantWhitelist) > 0 {
		if raw, err := json.Marshal(input.TenantWhitelist); err == nil {
			profile.TenantWhitelist = datatypes.JSON(raw)
		}
	}

	refs, sealed, err := s.sealCredentials(ctx, profile, input.Credentials)
	if err != nil {
		return nil, err
	}
	profile.SecretRefs = refs
	if sealed != nil {
		profile.SealedSecrets = sealed
	}

	result, err := s.repo.UpsertByScopeName(ctx, profile)
	if err != nil {
		return nil, err
	}
	s.cacheProviderHealth(ctx, result.UUID, result.HealthScore)
	s.emitAudit(ctx, "provider.registered", result, nil)
	s.inst.RecordMetric(ctx, "agent.provider.onboard_total", 1, map[string]string{
		"provider_id": result.UUID.String(),
		"status":      result.RolloutStatus,
	})
	return result, nil
}

// RotateEnv re-encrypts all provider secrets for an environment by rotating tenant key pairs.
func (s *Service) RotateEnv(ctx context.Context, env string) error {
	if s.repo == nil || s.keys == nil {
		return errors.New("rotation requires repository and tenant key service")
	}
	ctx, _ = instrumentation.EnsureTraceContext(ctx)
	entries, err := s.repo.ListByStatus(ctx, env, "", 0)
	if err != nil {
		return err
	}
	scopeBuckets := map[string][]model.ProviderProfile{}
	for _, prof := range entries {
		scope := scopeKey(prof.Env, prof.TenantID)
		scopeBuckets[scope] = append(scopeBuckets[scope], prof)
	}
	for _, providers := range scopeBuckets {
		if err := s.rotateScope(ctx, providers[0].Env, providers[0].TenantID, providers); err != nil {
			return err
		}
	}
	return nil
}

// RotateProvider rotates sealed secrets for a single provider UUID.
func (s *Service) RotateProvider(ctx context.Context, env string, tenantID *uint64, id uuid.UUID) error {
	profile, err := s.repo.FindByUUID(ctx, id)
	if err != nil {
		return err
	}
	if profile == nil {
		return fmt.Errorf("provider %s not found", id)
	}
	return s.rotateScope(ctx, env, tenantID, []model.ProviderProfile{*profile})
}

// StartSecretRotationJob launches a background ticker used by ops scripts / cron.
func (s *Service) StartSecretRotationJob(ctx context.Context, env string, interval time.Duration) {
	s.rotMgr.start(ctx, env, interval)
}

// StopSecretRotationJob stops the running ticker.
func (s *Service) StopSecretRotationJob() {
	s.rotMgr.stop()
}

// LoadProvider fetches a provider profile without exposing sealed payload.
func (s *Service) LoadProvider(ctx context.Context, id uuid.UUID) (*model.ProviderProfile, error) {
	return s.repo.FindByUUID(ctx, id)
}

func (s *Service) rotateScope(ctx context.Context, env string, tenantID *uint64, providers []model.ProviderProfile) error {
	if len(providers) == 0 {
		return nil
	}
	plainByProvider := map[uuid.UUID]map[string]any{}
	for _, prof := range providers {
		if len(prof.SealedSecrets) == 0 {
			continue
		}
		raw := map[string]any{}
		if err := s.keys.UnsealSensitive(ctx, env, tenantID, prof.SealedSecrets, &raw); err != nil {
			return fmt.Errorf("unseal %s: %w", prof.UUID, err)
		}
		plainByProvider[prof.UUID] = raw
	}
	if len(plainByProvider) == 0 {
		return nil
	}
	if _, err := s.keys.RotateKeyPair(ctx, env, tenantID); err != nil {
		return fmt.Errorf("rotate key pair: %w", err)
	}
	for _, prof := range providers {
		payload, ok := plainByProvider[prof.UUID]
		if !ok {
			continue
		}
		data := datatypes.JSONMap{}
		keys := make([]string, 0, len(payload))
		for k, v := range payload {
			data[k] = v
			keys = append(keys, k)
		}
		sealed, err := s.keys.SealSensitive(ctx, env, tenantID, data, keys...)
		if err != nil {
			return fmt.Errorf("seal secrets for %s: %w", prof.UUID, err)
		}
		if err := s.repo.UpdateSealedSecrets(ctx, prof.UUID, sealed); err != nil {
			return err
		}
		s.cacheRotationTimestamp(ctx, prof.UUID, s.clock())
		s.emitAudit(ctx, "provider.secret_rotated", &prof, nil)
		s.inst.RecordMetric(ctx, "agent.provider.secret_rotation_total", 1, map[string]string{
			"provider_id": prof.UUID.String(),
		})
	}
	return nil
}

func (s *Service) sealCredentials(ctx context.Context, profile *model.ProviderProfile, creds map[string]string) (datatypes.JSONMap, datatypes.JSONMap, error) {
	if len(creds) == 0 {
		return datatypes.JSONMap{}, nil, nil
	}
	raw := datatypes.JSONMap{}
	keys := make([]string, 0, len(creds))
	refs := datatypes.JSONMap{}
	for key, val := range creds {
		k := strings.TrimSpace(key)
		v := strings.TrimSpace(val)
		if k == "" || v == "" {
			continue
		}
		raw[k] = v
		keys = append(keys, k)
		refs[k] = fmt.Sprintf("vault://agent-model-hub/%s/%s/%s", profile.Env, profile.UUID.String(), strings.ToLower(k))
	}
	if len(keys) == 0 {
		return refs, nil, nil
	}
	sealed, err := s.keys.SealSensitive(ctx, profile.Env, profile.TenantID, raw, keys...)
	if err != nil {
		return nil, nil, err
	}
	return refs, sealed, nil
}

func (s *Service) cacheProviderHealth(ctx context.Context, id uuid.UUID, score float64) {
	if s.cache == nil {
		return
	}
	key := fmt.Sprintf(providerHealthCacheKey, id.String())
	payload := fmt.Sprintf("%.4f", score)
	if err := s.cache.Set(ctx, key, payload, 10*time.Minute); err != nil {
		logger.WarnF(ctx, "[provider_registry] cache health failed: %v", err)
	}
}

func (s *Service) cacheRotationTimestamp(ctx context.Context, id uuid.UUID, ts time.Time) {
	if s.cache == nil {
		return
	}
	key := fmt.Sprintf(providerRotationCacheKey, id.String())
	if err := s.cache.Set(ctx, key, ts.UTC().Format(time.RFC3339), 30*24*time.Hour); err != nil {
		logger.WarnF(ctx, "[provider_registry] cache rotation ts failed: %v", err)
	}
}

func (s *Service) emitAudit(ctx context.Context, operation string, prof *model.ProviderProfile, meta map[string]any) {
	if s.audit == nil || prof == nil {
		return
	}
	if meta == nil {
		meta = map[string]any{}
	}
	meta["provider_id"] = prof.UUID.String()
	meta["name"] = prof.Name
	meta["rollout_status"] = prof.RolloutStatus
	payload, _ := json.Marshal(meta)
	_ = s.audit.Emit(ctx, &dbmaudit.AuditEvent{
		Source:       "provider_registry.service",
		Operation:    operation,
		ResourceType: "agent.provider_profile",
		ResourceID:   prof.UUID.String(),
		Outcome:      "SUCCESS",
		Severity:     "INFO",
		Meta:         datatypes.JSON(payload),
		OccurredAt:   s.clock(),
	})
}

func statusOrDefault(status string) string {
	s := strings.TrimSpace(strings.ToLower(status))
	if s == "" {
		return "draft"
	}
	return s
}

func scopeKey(env string, tenantID *uint64) string {
	if tenantID == nil {
		return env + ":global"
	}
	return fmt.Sprintf("%s:%d", env, *tenantID)
}
