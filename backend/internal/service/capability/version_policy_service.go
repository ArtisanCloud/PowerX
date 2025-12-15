package capability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	dbmaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability"
	caprepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var (
	defaultStrategies = map[string]struct{}{
		"latest_minor": {},
		"fixed_major":  {},
		"custom":       {},
	}
	versionStatuses = map[string]struct{}{
		"active":     {},
		"deprecated": {},
		"blocked":    {},
	}

	// ErrPolicyValidation 表示版本策略校验失败。
	ErrPolicyValidation = errors.New("version policy validation failed")
)

// VersionPolicyService 负责版本策略的增删改查与校验。
type VersionPolicyService struct {
	db    *gorm.DB
	repo  *caprepo.VersionPolicyRepository
	audit auditsvc.Service
}

// NewVersionPolicyService 创建服务实例。
func NewVersionPolicyService(db *gorm.DB, audit auditsvc.Service) *VersionPolicyService {
	return &VersionPolicyService{
		db:    db,
		repo:  caprepo.NewVersionPolicyRepository(db),
		audit: audit,
	}
}

// VersionPolicy 对外暴露的策略结构。
type VersionPolicy struct {
	TenantUUID          string                 `json:"tenant_uuid"`
	CapabilityKey       string                 `json:"capability_key"`
	DefaultStrategy     string                 `json:"default_strategy"`
	AllowedVersions     []VersionRule          `json:"allowed_versions"`
	CompatibilityMatrix map[string]interface{} `json:"compatibility_matrix"`
	DeprecationPolicy   map[string]interface{} `json:"deprecation_policy"`
	AuditConfig         map[string]interface{} `json:"audit_config"`
	UpdatedAt           time.Time              `json:"updated_at"`
}

// VersionRule 描述单个版本的状态与兼容信息。
type VersionRule struct {
	Version        string   `json:"version"`
	CompatibleWith []string `json:"compatible_with"`
	Status         string   `json:"status"`
}

// VersionPolicyUpsertInput 版本策略写入参数。
type VersionPolicyUpsertInput struct {
	TenantUUID          string
	CapabilityKey       string
	DefaultStrategy     string
	AllowedVersions     []VersionRule
	CompatibilityMatrix map[string]interface{}
	DeprecationPolicy   map[string]interface{}
	AuditConfig         map[string]interface{}
}

// GetVersionPolicy 查询版本策略。
func (s *VersionPolicyService) GetVersionPolicy(ctx context.Context, tenantUUID string, capabilityKey string) (*VersionPolicy, error) {
	entity, err := s.repo.GetByKey(ctx, strings.TrimSpace(tenantUUID), capabilityKey)
	if err != nil {
		return nil, err
	}
	return toServicePolicy(entity)
}

// UpsertVersionPolicy 创建或更新策略。
func (s *VersionPolicyService) UpsertVersionPolicy(ctx context.Context, in *VersionPolicyUpsertInput) (*VersionPolicy, error) {
	if in == nil {
		return nil, errors.New("input cannot be nil")
	}
	if err := validatePolicyInput(in); err != nil {
		return nil, err
	}

	entity := &capmodel.CapabilityVersionPolicy{
		TenantUUID:          strings.TrimSpace(in.TenantUUID),
		CapabilityKey:       in.CapabilityKey,
		DefaultStrategy:     strings.ToLower(in.DefaultStrategy),
		AllowedVersions:     marshalJSON(in.AllowedVersions),
		CompatibilityMatrix: marshalJSON(in.CompatibilityMatrix),
		DeprecationPolicy:   marshalJSON(in.DeprecationPolicy),
		AuditConfig:         marshalJSON(in.AuditConfig),
		Status:              1,
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := s.repo.WithDB(tx)
		if _, err := repo.UpsertPolicy(ctx, entity); err != nil {
			return err
		}
		return s.emitAuditAndEvent(ctx, entity)
	})
	if err != nil {
		return nil, err
	}
	return toServicePolicy(entity)
}

func validatePolicyInput(in *VersionPolicyUpsertInput) error {
	if in.CapabilityKey == "" {
		return fmt.Errorf("%w: capability_key required", ErrPolicyValidation)
	}
	strategy := strings.ToLower(in.DefaultStrategy)
	if _, ok := defaultStrategies[strategy]; !ok {
		return fmt.Errorf("%w: unknown default_strategy %s", ErrPolicyValidation, in.DefaultStrategy)
	}
	if len(in.AllowedVersions) == 0 {
		return fmt.Errorf("%w: at least one allowed version required", ErrPolicyValidation)
	}
	for _, rule := range in.AllowedVersions {
		if rule.Version == "" {
			return fmt.Errorf("%w: allowed_versions.version empty", ErrPolicyValidation)
		}
		status := strings.ToLower(rule.Status)
		if status == "" {
			status = "active"
		}
		if _, ok := versionStatuses[status]; !ok {
			return fmt.Errorf("%w: invalid version status %s", ErrPolicyValidation, rule.Status)
		}
	}
	return nil
}

func toServicePolicy(entity *capmodel.CapabilityVersionPolicy) (*VersionPolicy, error) {
	if entity == nil {
		return nil, nil
	}
	return &VersionPolicy{
		TenantUUID:          entity.TenantUUID,
		CapabilityKey:       entity.CapabilityKey,
		DefaultStrategy:     entity.DefaultStrategy,
		AllowedVersions:     unmarshalVersionRules(entity.AllowedVersions),
		CompatibilityMatrix: unmarshalJSON(entity.CompatibilityMatrix),
		DeprecationPolicy:   unmarshalJSON(entity.DeprecationPolicy),
		AuditConfig:         unmarshalJSON(entity.AuditConfig),
		UpdatedAt:           entity.UpdatedAt,
	}, nil
}

func (s *VersionPolicyService) emitAuditAndEvent(ctx context.Context, entity *capmodel.CapabilityVersionPolicy) error {
	if s.audit != nil {
		tenantUUID := strings.TrimSpace(reqctx.GetTenantUUID(ctx))
		if tenantUUID == "" {
			tenantUUID = strings.TrimSpace(entity.TenantUUID)
		}
		payload := map[string]any{
			"capability_key":   entity.CapabilityKey,
			"default_strategy": entity.DefaultStrategy,
		}
		data, _ := json.Marshal(payload)
		_ = s.audit.Emit(ctx, &dbmaudit.AuditEvent{
			OccurredAt:   time.Now(),
			TenantUUID:   tenantUUID,
			Source:       "capability.version_policy",
			Operation:    "integration.capability.version_policy.updated",
			ResourceType: "capability.version_policy",
			ResourceID:   entity.CapabilityKey,
			Outcome:      "SUCCESS",
			Severity:     "INFO",
			Meta:         datatypes.JSON(data),
		})
	}
	event_bus.Publish(event_bus.Event{
		Name: "integration.capability.version_policy.updated",
		Payload: map[string]any{
			"tenant_uuid":      entity.TenantUUID,
			"capability_key":   entity.CapabilityKey,
			"default_strategy": entity.DefaultStrategy,
		},
		Ctx: ctx,
	})
	return nil
}

func marshalJSON(v interface{}) datatypes.JSON {
	if v == nil {
		return datatypes.JSON([]byte("{}"))
	}
	data, err := json.Marshal(v)
	if err != nil {
		return datatypes.JSON([]byte("{}"))
	}
	return datatypes.JSON(data)
}

func unmarshalJSON(data datatypes.JSON) map[string]interface{} {
	if len(data) == 0 {
		return map[string]interface{}{}
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]interface{}{}
	}
	return out
}

func unmarshalVersionRules(data datatypes.JSON) []VersionRule {
	if len(data) == 0 {
		return nil
	}
	var out []VersionRule
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}
