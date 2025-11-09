package connector_guard

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

	instance := &model.ConnectorInstance{
		Env:                  env,
		TenantScope:          scope,
		Platform:             platform,
		Region:               strings.TrimSpace(input.Region),
		OAuthRef:             strings.TrimSpace(input.OAuthRef),
		WebhookSigningKeyRef: strings.TrimSpace(input.WebhookSigningKeyRef),
		MappingTemplate:      input.MappingTemplate,
		Status:               sanitizeStatus(input.Status, "active"),
		RateLimitPerMinute:   input.RateLimitPerMinute,
	}
	instance.UUID = uuid.New()

	if sealed, refs, err := s.sealConnectorSecrets(ctx, instance, input.Secrets); err != nil {
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
func (s *Service) PauseInstance(ctx context.Context, id uuid.UUID, reason string) error {
	if err := s.repo.UpdateStatus(ctx, id, "paused", reason); err != nil {
		return err
	}
	s.cacheStatus(ctx, id, "paused")
	s.inst.RecordMetric(ctx, "agent.connector.pause_total", 1, map[string]string{})
	s.emitAudit(ctx, "connector.instance.paused", &model.ConnectorInstance{
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
	if v == "" {
		return def
	}
	return v
}
