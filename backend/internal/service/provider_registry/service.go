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
	"github.com/ArtisanCloud/PowerX/pkg/utils"
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
	db        *gorm.DB
	cache     cache.ICache
	audit     audit.Service
	keys      *tenantkeys.TenantKeyService
	repo      *repo.ProviderProfileRepository
	inst      *instrumentation.Instrumentation
	clock     func() time.Time
	rotMgr    *secretRotationScheduler
	artifacts *validationArtifactStore
}

// Options wires ProviderRegistry dependencies.
type Options struct {
	shared.Options
	Profiles  *repo.ProviderProfileRepository
	Artifacts ValidationArtifactOptions
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
	svc.artifacts = newValidationArtifactStore(opts.Artifacts, svc.clock)
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

// PublishOptions controls rollout behavior.
type PublishOptions struct {
	TenantWhitelist       []TenantRef
	RolloutStrategy       string
	RollbackTimeoutMinute uint32
}

var (
	ErrProviderNotFound = errors.New("provider not found")
	ErrValidationFailed = errors.New("provider validation failed")
)

// RegisterProvider stores or updates a provider profile using Vault-style sealed secrets.
func (s *Service) RegisterProvider(ctx context.Context, env string, tenantID *uint64, input ProviderProfileInput) (*model.ProviderProfile, error) {
	ctx, _ = instrumentation.EnsureTraceContext(ctx)
	start := s.clock()
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
		Metadata:        datatypes.JSONMap{},
	}
	profile.UUID = uuid.New()
	if raw, err := marshalTenantWhitelist(input.TenantWhitelist); err == nil {
		profile.TenantWhitelist = raw
	} else if len(input.TenantWhitelist) > 0 {
		return nil, fmt.Errorf("tenantWhitelist marshal: %w", err)
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
	duration := s.clock().Sub(start).Seconds()
	s.inst.RecordMetric(ctx, "agent.provider.onboard_duration", duration, map[string]string{
		"provider_id": result.UUID.String(),
		"env":         result.Env,
	})
	return result, nil
}

// ValidateProvider runs automated checks and records health metadata.
func (s *Service) ValidateProvider(ctx context.Context, providerID uuid.UUID, suite string, report *ValidationReport) (*model.ProviderProfile, error) {
	ctx, _ = instrumentation.EnsureTraceContext(ctx)
	if strings.TrimSpace(suite) == "" {
		return nil, errors.New("validation suite required")
	}
	profile, err := s.mustProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}

	if report != nil {
		report.Normalize(providerID, suite, s.clock)
	}
	status := validationStatusUnknown
	if report != nil {
		status = report.Status()
	}

	var artifactMeta datatypes.JSONMap
	var sealed datatypes.JSONMap
	if report != nil && s.artifacts != nil {
		payload, err := report.MarshalJSONBytes()
		if err != nil {
			return nil, fmt.Errorf("marshal validation report: %w", err)
		}
		record, err := s.artifacts.Save(ctx, profile.UUID, suite, payload)
		if err != nil {
			return nil, err
		}
		artifactMeta = artifactMetaToJSON(record, report, status)
		vaultRef := fmt.Sprintf("vault://agent/providers/%s/%s", profile.UUID.String(), record.StoredAt.UTC().Format("20060102T150405Z"))
		sealedPayload := datatypes.JSONMap{
			"vault_ref":      vaultRef,
			"artifact_uri":   record.URI,
			"checksum":       record.Checksum,
			"suite":          report.Suite,
			"status":         status,
			"stored_at":      record.StoredAt.UTC().Format(time.RFC3339),
			"generated_at":   report.GeneratedAt,
			"provider_id":    profile.UUID.String(),
			"artifact_stats": report.StatsMap(),
		}
		var keys []string
		for k := range sealedPayload {
			keys = append(keys, k)
		}
		sealedMap, err := s.keys.SealSensitive(ctx, profile.Env, profile.TenantID, sealedPayload, keys...)
		if err != nil {
			return nil, fmt.Errorf("seal validation artifact ref: %w", err)
		}
		sealed = sealedMap
		if artifactMeta != nil {
			artifactMeta["vault_ref"] = vaultRef
		}
	}

	meta := utils.CloneJSONMap(profile.Metadata)
	meta["validation_suite"] = suite
	meta["validated_at"] = s.clock().UTC().Format(time.RFC3339)
	if artifactMeta != nil {
		meta["validation_artifact"] = artifactMeta
	}
	if sealed != nil {
		meta["validation_vault"] = sealed
	}
	statusMeta := datatypes.JSONMap{
		"status":     status,
		"checked_at": meta["validated_at"],
		"suite":      suite,
	}
	if report != nil {
		if stats := report.StatsMap(); stats != nil {
			statusMeta["stats"] = stats
		}
		if strings.TrimSpace(report.GeneratedAt) != "" {
			statusMeta["generated_at"] = report.GeneratedAt
		}
	}
	meta["validation_status"] = statusMeta

	health := profile.HealthScore
	switch status {
	case validationStatusPass:
		if health < 0.95 {
			health = 0.98
		}
	case validationStatusFail:
		if health > 0.4 {
			health = 0.4
		}
	default:
		if health < 0.9 {
			health = 0.95
		}
	}
	updates := map[string]any{
		"rollout_status": "validating",
		"health_score":   health,
		"metadata":       meta,
	}
	if err := s.repo.UpdateFields(ctx, providerID, updates); err != nil {
		return nil, err
	}
	profile.RolloutStatus = "validating"
	profile.HealthScore = health
	profile.Metadata = meta
	s.emitAudit(ctx, "provider.validated", profile, map[string]any{"suite": suite, "status": status})
	s.inst.RecordMetric(ctx, "agent.provider.validation_total", 1, map[string]string{
		"provider_id": profile.UUID.String(),
		"suite":       suite,
		"status":      status,
	})
	s.inst.RecordMetric(ctx, "agent.provider.health_score", health, map[string]string{
		"provider_id": profile.UUID.String(),
		"suite":       suite,
		"status":      status,
	})
	return profile, nil
}

// PublishProvider applies rollout strategy & tenant whitelist updates.
func (s *Service) PublishProvider(ctx context.Context, providerID uuid.UUID, opts PublishOptions) (*model.ProviderProfile, error) {
	ctx, _ = instrumentation.EnsureTraceContext(ctx)
	profile, err := s.mustProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if status := currentValidationStatus(profile.Metadata); status == validationStatusFail {
		return nil, fmt.Errorf("%w: suite %s failed", ErrValidationFailed, profile.Metadata["validation_suite"])
	}

	meta := utils.CloneJSONMap(profile.Metadata)
	strategy := sanitizeStrategy(opts.RolloutStrategy)
	meta["rollout_strategy"] = strategy
	if opts.RollbackTimeoutMinute > 0 {
		meta["rollback_timeout_minutes"] = opts.RollbackTimeoutMinute
	}
	meta["published_at"] = s.clock().UTC().Format(time.RFC3339)

	status := "gray"
	if strategy == "full" {
		status = "live"
	}

	updates := map[string]any{
		"rollout_status": status,
		"metadata":       meta,
	}
	if len(opts.TenantWhitelist) > 0 {
		raw, err := marshalTenantWhitelist(opts.TenantWhitelist)
		if err != nil {
			return nil, fmt.Errorf("tenantWhitelist marshal: %w", err)
		}
		updates["tenant_whitelist"] = raw
		profile.TenantWhitelist = raw
	}
	if err := s.repo.UpdateFields(ctx, providerID, updates); err != nil {
		return nil, err
	}
	profile.RolloutStatus = status
	profile.Metadata = meta
	s.emitAudit(ctx, "provider.published", profile, map[string]any{
		"strategy": strategy,
	})
	s.inst.RecordMetric(ctx, "agent.provider.publish_total", 1, map[string]string{
		"provider_id": profile.UUID.String(),
		"strategy":    strategy,
	})
	return profile, nil
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
			"env":         prof.Env,
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

func (s *Service) mustProvider(ctx context.Context, id uuid.UUID) (*model.ProviderProfile, error) {
	profile, err := s.repo.FindByUUID(ctx, id)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, ErrProviderNotFound
	}
	return profile, nil
}

func sanitizeStrategy(strategy string) string {
	switch strings.ToLower(strings.TrimSpace(strategy)) {
	case "full":
		return "full"
	case "percentage":
		return "percentage"
	case "canary":
		return "canary"
	default:
		return "gray"
	}
}

func marshalTenantWhitelist(refs []TenantRef) (datatypes.JSON, error) {
	if len(refs) == 0 {
		return datatypes.JSON([]byte("[]")), nil
	}
	b, err := json.Marshal(refs)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(b), nil
}

// DecodeTenantWhitelist converts stored JSON into typed refs.
func DecodeTenantWhitelist(raw datatypes.JSON) []TenantRef {
	if len(raw) == 0 {
		return nil
	}
	var refs []TenantRef
	if err := json.Unmarshal(raw, &refs); err != nil {
		return nil
	}
	return refs
}

func currentValidationStatus(meta datatypes.JSONMap) string {
	if meta == nil {
		return validationStatusUnknown
	}
	raw, ok := meta["validation_status"]
	if !ok || raw == nil {
		return validationStatusUnknown
	}
	switch v := raw.(type) {
	case string:
		return strings.ToLower(strings.TrimSpace(v))
	case map[string]any:
		if status, ok := v["status"].(string); ok {
			return strings.ToLower(strings.TrimSpace(status))
		}
	case datatypes.JSONMap:
		if status, ok := v["status"].(string); ok {
			return strings.ToLower(strings.TrimSpace(status))
		}
	}
	return validationStatusUnknown
}
