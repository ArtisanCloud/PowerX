package connector_guard

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/instrumentation"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/shared"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	"github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent_model_hub"
	dbmaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/agent_model_hub"
	"github.com/ArtisanCloud/PowerX/pkg/corex/tenantkeys"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const connectorStatusCacheKey = "agent:modelhub:connector:%s:status"

var (
	supportedConnectorPlatforms = map[string]struct{}{
		"coze": {},
		"n8n":  {},
	}
	allowedConnectorStatuses = map[string]struct{}{
		"active":    {},
		"paused":    {},
		"degrading": {},
	}
)

const (
	mappingTemplateMaxBytes = 64 * 1024
	defaultSignatureDrift   = 5 * time.Minute
	errorRateAlpha          = 0.2
	degradeSourceManual     = "manual"
	degradeSourceAuto       = "auto"
)

var (
	ErrConnectorNotFound = errors.New("connector instance not found")
	ErrSignatureMismatch = errors.New("invalid webhook signature")
	ErrSigningKeyMissing = errors.New("webhook signing key missing")
)

// Service coordinates connector registration, pause/resume, and secret sealing.
type Service struct {
	db    *gorm.DB
	cache cache.ICache
	audit audit.Service
	keys  *tenantkeys.TenantKeyService
	repo  *repo.ConnectorInstanceRepository
	inst  *instrumentation.Instrumentation
	clock func() time.Time
}

type Options struct {
	shared.Options
	Instances *repo.ConnectorInstanceRepository
}

// WebhookVerificationInput defines signature verification payload.
type WebhookVerificationInput struct {
	InstanceID uuid.UUID
	Signature  string
	Timestamp  string
	Payload    []byte
	MaxDrift   time.Duration
}

// CallbackMetricInput describes success/failure outcomes for auto-pause logic.
type CallbackMetricInput struct {
	InstanceID uuid.UUID
	Success    bool
	Threshold  float64
	Reason     string
	LatencyMs  float64
}

func NewService(opts Options) *Service {
	opts.Options.Normalize()
	r := opts.Instances
	if r == nil && opts.DB != nil {
		r = repo.NewConnectorInstanceRepository(opts.DB)
	}
	return &Service{
		db:    opts.DB,
		cache: opts.Cache,
		audit: opts.AuditSvc,
		keys:  opts.TenantKeySvc,
		repo:  r,
		inst:  opts.Instrumentation,
		clock: opts.Clock,
	}
}

// ConnectorInstanceInput represents connector upsert payloads.
type ConnectorInstanceInput struct {
	TenantScope          string
	Platform             string
	Region               string
	OAuthRef             string
	WebhookSigningKeyRef string
	MappingTemplate      datatypes.JSON
	RateLimitPerMinute   uint32
	Status               string
	Secrets              map[string]string
	InstanceID           string
}

// UpsertInstance registers or updates a connector instance while sealing optional secrets.
func (s *Service) UpsertInstance(ctx context.Context, env string, input ConnectorInstanceInput) (*model.ConnectorInstance, error) {
	if s.repo == nil {
		return nil, errors.New("connector repository is not configured")
	}
	scope := strings.TrimSpace(input.TenantScope)
	if scope == "" {
		return nil, errors.New("tenant scope required")
	}
	platform := strings.ToLower(strings.TrimSpace(input.Platform))
	if platform == "" {
		return nil, errors.New("platform required")
	}
	if _, ok := supportedConnectorPlatforms[platform]; !ok {
		return nil, fmt.Errorf("unsupported platform %s", platform)
	}

	template, err := normalizeMappingTemplate(input.MappingTemplate)
	if err != nil {
		return nil, err
	}

	secrets := sanitizeSecretMap(input.Secrets)
	if strings.TrimSpace(input.OAuthRef) == "" && secrets["oauth_token"] == "" {
		return nil, errors.New("oauthRef or oauth_token secret required")
	}
	if strings.TrimSpace(input.WebhookSigningKeyRef) == "" && secrets["webhook_signing_key"] == "" {
		return nil, errors.New("webhookSigningKeyRef or webhook_signing_key secret required")
	}

	var instanceID uuid.UUID
	if trimmed := strings.TrimSpace(input.InstanceID); trimmed != "" {
		parsed, err := uuid.Parse(trimmed)
		if err != nil {
			return nil, fmt.Errorf("invalid instance_id: %w", err)
		}
		instanceID = parsed
	} else {
		instanceID = uuid.New()
	}

	instance := &model.ConnectorInstance{
		PowerUUIDModel: coremodel.PowerUUIDModel{
			UUID: instanceID,
		},
		Env:                  env,
		TenantScope:          scope,
		Platform:             platform,
		Region:               strings.TrimSpace(input.Region),
		OAuthRef:             strings.TrimSpace(input.OAuthRef),
		WebhookSigningKeyRef: strings.TrimSpace(input.WebhookSigningKeyRef),
		MappingTemplate:      template,
		Status:               sanitizeStatus(input.Status, "active"),
		RateLimitPerMinute:   input.RateLimitPerMinute,
	}

	if sealed, refs, err := s.sealConnectorSecrets(ctx, instance, secrets); err != nil {
		return nil, err
	} else if sealed != nil {
		instance.SealedSecrets = sealed
		if refs["oauth_token"] != "" && instance.OAuthRef == "" {
			instance.OAuthRef = refs["oauth_token"]
		}
		if refs["webhook_signing_key"] != "" && instance.WebhookSigningKeyRef == "" {
			instance.WebhookSigningKeyRef = refs["webhook_signing_key"]
		}
	}

	result, err := s.repo.Upsert(ctx, instance)
	if err != nil {
		return nil, err
	}
	s.cacheStatus(ctx, result.UUID, result.Status)
	s.emitAudit(ctx, "connector.instance.upsert", result, nil)
	s.inst.RecordMetric(ctx, "agent.connector.instance_total", 1, map[string]string{
		"platform":     result.Platform,
		"tenant_scope": result.TenantScope,
		"status":       result.Status,
	})
	return result, nil
}

// PauseInstance updates status and audit trail for degraded connectors.
func (s *Service) PauseInstance(ctx context.Context, id uuid.UUID, reason string, source string) error {
	ctx, _ = instrumentation.EnsureTraceContext(ctx)
	if strings.TrimSpace(source) == "" {
		source = degradeSourceManual
	}
	inst, err := s.repo.FindByUUID(ctx, id)
	if err != nil {
		return err
	}
	if inst == nil {
		return ErrConnectorNotFound
	}
	if err := s.repo.UpdateStatus(ctx, id, "paused", reason); err != nil {
		return err
	}
	inst.Status = "paused"
	inst.LastPauseReason = reason
	s.cacheStatus(ctx, id, "paused")
	pauseLabels := map[string]string{
		"platform":     inst.Platform,
		"tenant_scope": inst.TenantScope,
		"instance_id":  inst.UUID.String(),
	}
	s.inst.RecordMetric(ctx, "agent.connector.pause_total", 1, pauseLabels)
	degradeLabels := map[string]string{
		"platform":     inst.Platform,
		"tenant_scope": inst.TenantScope,
		"instance_id":  inst.UUID.String(),
		"source":       source,
	}
	s.inst.RecordMetric(ctx, "agent.platform.degrade_total", 1, degradeLabels)
	s.emitAudit(ctx, "connector.instance.paused", inst, map[string]any{"reason": reason})
	return nil
}

// ResumeInstance brings a paused connector back to active state.
func (s *Service) ResumeInstance(ctx context.Context, id uuid.UUID, reason string) error {
	if err := s.repo.UpdateStatus(ctx, id, "active", reason); err != nil {
		return err
	}
	s.cacheStatus(ctx, id, "active")
	s.inst.RecordMetric(ctx, "agent.connector.resume_total", 1, map[string]string{})
	s.emitAudit(ctx, "connector.instance.resumed", &model.ConnectorInstance{
		PowerUUIDModel: coremodel.PowerUUIDModel{UUID: id},
	}, map[string]any{"reason": reason})
	return nil
}

// RotateInstanceSecrets re-encrypts connector secrets with a refreshed tenant key.
func (s *Service) RotateInstanceSecrets(ctx context.Context, env, tenantScope string, id uuid.UUID) error {
	if s.keys == nil {
		return errors.New("tenant key service required for rotation")
	}
	instance, err := s.repo.FindByUUID(ctx, id)
	if err != nil {
		return err
	}
	if instance == nil || len(instance.SealedSecrets) == 0 {
		return nil
	}
	secrets := map[string]any{}
	if err := s.keys.UnsealSensitive(ctx, env, nil, instance.SealedSecrets, &secrets); err != nil {
		return err
	}
	data := datatypes.JSONMap{}
	keys := make([]string, 0, len(secrets))
	for k, v := range secrets {
		data[k] = v
		keys = append(keys, k)
	}
	if _, err := s.keys.RotateKeyPair(ctx, env, nil); err != nil {
		return err
	}
	sealed, err := s.keys.SealSensitive(ctx, env, nil, data, keys...)
	if err != nil {
		return err
	}
	if err := s.repo.UpdateSealedSecrets(ctx, instance.UUID, sealed); err != nil {
		return err
	}
	s.emitAudit(ctx, "connector.instance.secret_rotated", instance, nil)
	return nil
}

// TrackCallbackMetric updates rolling error rates and auto-pauses if thresholds are crossed.
func (s *Service) TrackCallbackMetric(ctx context.Context, input CallbackMetricInput) (float64, bool, error) {
	ctx, _ = instrumentation.EnsureTraceContext(ctx)
	if s.repo == nil {
		return 0, false, errors.New("connector repository is not configured")
	}
	inst, err := s.repo.FindByUUID(ctx, input.InstanceID)
	if err != nil {
		return 0, false, err
	}
	if inst == nil {
		return 0, false, ErrConnectorNotFound
	}
	ctx = instrumentation.WithTenant(ctx, inst.TenantScope)
	labels := map[string]string{
		"platform":     inst.Platform,
		"tenant_scope": inst.TenantScope,
		"instance_id":  inst.UUID.String(),
	}
	if input.LatencyMs > 0 {
		s.inst.RecordMetric(ctx, "agent.platform.latency_p95", input.LatencyMs, labels)
	}
	if !input.Success {
		failLabels := map[string]string{}
		for k, v := range labels {
			failLabels[k] = v
		}
		if trimmed := strings.TrimSpace(input.Reason); trimmed != "" {
			failLabels["reason"] = trimmed
		}
		s.inst.RecordMetric(ctx, "agent.platform.callback_failure_total", 1, failLabels)
	}
	fail := 0.0
	if !input.Success {
		fail = 1
	}
	newRate := clampRate(fail*errorRateAlpha + inst.ErrorRate*(1-errorRateAlpha))
	if err := s.repo.UpdateErrorRate(ctx, inst.UUID, newRate); err != nil {
		return 0, false, err
	}
	threshold := input.Threshold
	triggered := false
	if threshold > 0 && newRate >= threshold && strings.ToLower(inst.Status) != "paused" {
		triggered = true
		reason := input.Reason
		if reason == "" {
			reason = fmt.Sprintf("auto-pause: error_rate %.4f ≥ %.4f", newRate, threshold)
		}
		if err := s.PauseInstance(ctx, inst.UUID, reason, degradeSourceAuto); err != nil {
			return newRate, false, err
		}
	}
	return newRate, triggered, nil
}

func (s *Service) sealConnectorSecrets(ctx context.Context, inst *model.ConnectorInstance, secrets map[string]string) (datatypes.JSONMap, map[string]string, error) {
	if len(secrets) == 0 || s.keys == nil {
		return nil, map[string]string{}, nil
	}
	raw := datatypes.JSONMap{}
	keys := make([]string, 0, len(secrets))
	refs := map[string]string{}
	for k, v := range secrets {
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(v)
		if key == "" || val == "" {
			continue
		}
		raw[key] = val
		keys = append(keys, key)
		switch key {
		case "oauth_token":
			refs[key] = fmt.Sprintf("vault://agent-model-hub/%s/connector/%s/oauth", inst.Env, inst.UUID.String())
		case "webhook_signing_key":
			refs[key] = fmt.Sprintf("vault://agent-model-hub/%s/connector/%s/webhook", inst.Env, inst.UUID.String())
		}
	}
	if len(keys) == 0 {
		return nil, refs, nil
	}
	sealed, err := s.keys.SealSensitive(ctx, inst.Env, nil, raw, keys...)
	if err != nil {
		return nil, nil, err
	}
	return sealed, refs, nil
}

func (s *Service) cacheStatus(ctx context.Context, id uuid.UUID, status string) {
	if s.cache == nil {
		return
	}
	key := fmt.Sprintf(connectorStatusCacheKey, id.String())
	if err := s.cache.Set(ctx, key, status, 5*time.Minute); err != nil {
		logger.WarnF(ctx, "[connector_guard] cache status failed: %v", err)
	}
}

func (s *Service) emitAudit(ctx context.Context, op string, instModel *model.ConnectorInstance, meta map[string]any) {
	if s.audit == nil || instModel == nil {
		return
	}
	if meta == nil {
		meta = map[string]any{}
	}
	meta["platform"] = instModel.Platform
	meta["tenant_scope"] = instModel.TenantScope
	payload, _ := json.Marshal(meta)
	_ = s.audit.Emit(ctx, &dbmaudit.AuditEvent{
		Source:       "connector_guard.service",
		Operation:    op,
		ResourceType: "agent.connector_instance",
		ResourceID:   instModel.UUID.String(),
		Outcome:      "SUCCESS",
		Severity:     "INFO",
		Meta:         datatypes.JSON(payload),
		OccurredAt:   s.clock(),
	})
}

func sanitizeStatus(in string, def string) string {
	v := strings.TrimSpace(strings.ToLower(in))
	if _, ok := allowedConnectorStatuses[v]; ok {
		return v
	}
	return def
}

// VerifyWebhookSignature ensures callbacks include a valid signature + timestamp.
func (s *Service) VerifyWebhookSignature(ctx context.Context, input WebhookVerificationInput) error {
	if s.repo == nil {
		return errors.New("connector repository is not configured")
	}
	if input.InstanceID == uuid.Nil {
		return errors.New("instance_id required")
	}
	if strings.TrimSpace(input.Signature) == "" {
		return errors.New("signature required")
	}
	if strings.TrimSpace(input.Timestamp) == "" {
		return errors.New("timestamp required")
	}
	inst, err := s.repo.FindByUUID(ctx, input.InstanceID)
	if err != nil {
		return err
	}
	if inst == nil {
		return ErrConnectorNotFound
	}
	secrets, err := s.loadSecrets(ctx, inst)
	if err != nil {
		return err
	}
	key := strings.TrimSpace(secrets["webhook_signing_key"])
	if key == "" {
		return ErrSigningKeyMissing
	}
	ts, err := parseTimestamp(input.Timestamp)
	if err != nil {
		return err
	}
	maxDrift := input.MaxDrift
	if maxDrift <= 0 {
		maxDrift = defaultSignatureDrift
	}
	now := s.clock().UTC()
	drift := now.Sub(ts)
	if drift < 0 {
		drift = -drift
	}
	if drift > maxDrift {
		return fmt.Errorf("callback timestamp drift %s exceeds %s", drift, maxDrift)
	}

	expected := computeSignature(key, input.Timestamp, input.Payload)
	provided := normalizeSignature(input.Signature)
	if subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
		return ErrSignatureMismatch
	}
	return nil
}

func (s *Service) loadSecrets(ctx context.Context, inst *model.ConnectorInstance) (map[string]string, error) {
	if inst == nil || inst.SealedSecrets == nil {
		return map[string]string{}, nil
	}
	if s.keys == nil {
		return nil, errors.New("tenant key service not configured")
	}
	secrets := map[string]string{}
	if err := s.keys.UnsealSensitive(ctx, inst.Env, nil, inst.SealedSecrets, &secrets); err != nil {
		return nil, err
	}
	return secrets, nil
}

func computeSignature(secret string, timestamp string, payload []byte) string {
	body := append([]byte(timestamp), '.')
	body = append(body, payload...)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func normalizeSignature(sig string) string {
	s := strings.TrimSpace(sig)
	s = strings.TrimPrefix(s, "sha256=")
	return strings.ToLower(s)
}

func parseTimestamp(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, errors.New("timestamp required")
	}
	if unix, err := strconv.ParseInt(raw, 10, 64); err == nil {
		// handle millisecond precision if length > 11
		if len(raw) > 11 {
			return time.UnixMilli(unix).UTC(), nil
		}
		return time.Unix(unix, 0).UTC(), nil
	}
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid timestamp: %w", err)
	}
	return ts.UTC(), nil
}

func sanitizeSecretMap(secrets map[string]string) map[string]string {
	if len(secrets) == 0 {
		return nil
	}
	out := make(map[string]string, len(secrets))
	for k, v := range secrets {
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(v)
		if key == "" || val == "" {
			continue
		}
		out[key] = val
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeMappingTemplate(raw datatypes.JSON) (datatypes.JSON, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return datatypes.JSON([]byte("{}")), nil
	}
	if len(raw) > mappingTemplateMaxBytes {
		return nil, fmt.Errorf("mapping template exceeds %d bytes", mappingTemplateMaxBytes)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("mapping template must be a JSON object: %w", err)
	}
	for key := range obj {
		if strings.TrimSpace(key) == "" {
			return nil, errors.New("mapping template keys cannot be empty")
		}
	}
	buf, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(buf), nil
}

func clampRate(val float64) float64 {
	return math.Max(0, math.Min(1, val))
}
