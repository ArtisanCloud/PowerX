package capability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/ArtisanCloud/PowerX/internal/contract/capability"
	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	dbmaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability"
	caprepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

// ErrValidation 标识契约校验失败。
var ErrValidation = errors.New("capability contract validation failed")

// ContractService 负责契约的创建、校验、发布与查询。
type ContractService struct {
	db            *gorm.DB
	validator     *capability.Validator
	contractRepo  *caprepo.ContractRepository
	transportRepo *caprepo.TransportProfileRepository
	audit         auditsvc.Service
}

// NewContractService 创建服务实例。
func NewContractService(db *gorm.DB, validator *capability.Validator, audit auditsvc.Service) *ContractService {
	return &ContractService{
		db:            db,
		validator:     validator,
		contractRepo:  caprepo.NewContractRepository(db),
		transportRepo: caprepo.NewTransportProfileRepository(db),
		audit:         audit,
	}
}

// Contract 定义对外返回的契约信息。
type Contract struct {
	ID                    uint64                           `json:"id"`
	ContractUUID          string                           `json:"contract_uuid"`
	TenantID              uint64                           `json:"tenant_id"`
	CapabilityKey         string                           `json:"capability_key"`
	Version               string                           `json:"version"`
	ProviderID            string                           `json:"provider_id"`
	DisplayName           string                           `json:"display_name"`
	Description           string                           `json:"description,omitempty"`
	LifecycleState        string                           `json:"lifecycle_state"`
	SecurityScope         string                           `json:"security_scope"`
	ToolGrantRequired     bool                             `json:"tool_grant_required"`
	ObservabilityConfig   map[string]interface{}           `json:"observability_config,omitempty"`
	TransportPreferences  []capability.TransportPreference `json:"transport_preferences,omitempty"`
	IOSchemas             []capability.IOSchemaDescriptor  `json:"io_schemas,omitempty"`
	ErrorTaxonomy         []capability.ErrorTaxonomyEntry  `json:"error_taxonomy,omitempty"`
	TransportProfiles     []capability.TransportProfile    `json:"transport_profiles,omitempty"`
	EffectiveAt           *time.Time                       `json:"effective_at,omitempty"`
	DeprecatedAt          *time.Time                       `json:"deprecated_at,omitempty"`
	ReplacementCapability string                           `json:"replacement_capability,omitempty"`
	CreatedAt             time.Time                        `json:"created_at"`
	UpdatedAt             time.Time                        `json:"updated_at"`
}

// ContractUpsertInput 描述创建或更新草稿的入参。
type ContractUpsertInput struct {
	TenantID             uint64
	CapabilityKey        string
	Version              string
	ProviderID           string
	DisplayName          string
	Description          string
	SecurityScope        string
	ToolGrantRequired    bool
	ObservabilityConfig  map[string]interface{}
	IOSchemas            []capability.IOSchemaDescriptor
	TransportPreferences []capability.TransportPreference
	TransportProfiles    []capability.TransportProfile
	ErrorTaxonomy        []capability.ErrorTaxonomyEntry
}

// PublishInput 提交发布所需参数。
type PublishInput struct {
	TenantID      uint64
	CapabilityKey string
	Version       string
	EffectiveAt   time.Time
	Notes         string
}

// DeprecateInput 提交废弃所需参数。
type DeprecateInput struct {
	TenantID              uint64
	CapabilityKey         string
	Version               string
	DeprecatedAt          time.Time
	ReplacementCapability string
	AdvisoryMessage       string
}

// UpsertDraft 创建或更新契约草稿。
func (s *ContractService) UpsertDraft(ctx context.Context, in *ContractUpsertInput) (*Contract, []capability.ValidationIssue, error) {
	if in == nil {
		return nil, nil, errors.New("input cannot be nil")
	}

	issues, err := s.validate(ctx, in)
	if err != nil {
		return nil, issues, err
	}

	entity := &capmodel.CapabilityContract{
		TenantID:             in.TenantID,
		CapabilityKey:        in.CapabilityKey,
		Version:              in.Version,
		ProviderID:           in.ProviderID,
		DisplayName:          in.DisplayName,
		Description:          in.Description,
		SecurityScope:        in.SecurityScope,
		ToolGrantRequired:    in.ToolGrantRequired,
		LifecycleState:       "draft",
		ObservabilityConfig:  toJSON(in.ObservabilityConfig),
		TransportPreferences: toJSON(in.TransportPreferences),
		Status:               1,
	}

	err = s.db.WithContext(ctx).
		// Debug().
		Transaction(func(tx *gorm.DB) error {
			repo := s.contractRepo.WithDB(tx)
			entity, err = repo.UpsertContract(ctx, entity)
			if err != nil {
				logger.ErrorF(ctx, "[capability] upsert contract failed: %v", err)
				return err
			}

			if err := repo.ReplaceIOSchemas(ctx, entity.ID, toModelIOSchemas(in.TenantID, entity.ID, in.IOSchemas)); err != nil {
				logger.ErrorF(ctx, "[capability] replace io schemas failed: %v", err)
				return err
			}
			errorBindings, err := s.ensureErrorTaxonomy(ctx, tx, in.TenantID, entity.ID, in.ErrorTaxonomy)
			if err != nil {
				logger.ErrorF(ctx, "[capability] ensure error taxonomy failed: %v", err)
				return err
			}
			if err := repo.ReplaceErrorBindings(ctx, entity.ID, errorBindings); err != nil {
				logger.ErrorF(ctx, "[capability] replace error bindings failed: %v", err)
				return err
			}

			if err := s.transportRepo.WithDB(tx).DeleteByContract(ctx, entity.ID); err != nil {
				logger.ErrorF(ctx, "[capability] delete transport profiles failed: %v", err)
				return err
			}
			if len(in.TransportProfiles) > 0 {
				if err := s.transportRepo.WithDB(tx).UpsertProfiles(ctx, toModelTransportProfiles(in.TenantID, entity.ID, in.CapabilityKey, in.TransportProfiles)); err != nil {
					logger.ErrorF(ctx, "[capability] upsert transport profiles failed: %v", err)
					return err
				}
			}
			return nil
		})
	if err != nil {
		return nil, issues, err
	}

	res, buildErr := s.buildContract(ctx, entity)
	if buildErr != nil {
		return nil, issues, buildErr
	}
	return res, issues, nil
}

// GetContract 获取单个契约。
func (s *ContractService) GetContract(ctx context.Context, tenantID uint64, capabilityKey, version string) (*Contract, error) {
	entity, err := s.contractRepo.FindByKeyVersion(ctx, tenantID, capabilityKey, version, true)
	if err != nil {
		return nil, err
	}
	return s.buildContract(ctx, entity)
}

// ListContracts 以简单分页方式列出契约。
func (s *ContractService) ListContracts(ctx context.Context, tenantID uint64, keyword string, limit, offset int) ([]*Contract, int64, error) {
	entities, total, err := s.contractRepo.ListContracts(ctx, tenantID, keyword, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	results := make([]*Contract, 0, len(entities))
	for i := range entities {
		c, err := s.buildContract(ctx, &entities[i])
		if err != nil {
			return nil, 0, err
		}
		results = append(results, c)
	}
	return results, total, nil
}

// PublishContract 校验并发布契约。
func (s *ContractService) PublishContract(ctx context.Context, in *PublishInput) (*Contract, []capability.ValidationIssue, error) {
	entity, err := s.contractRepo.FindByKeyVersion(ctx, in.TenantID, in.CapabilityKey, in.Version, true)
	if err != nil {
		return nil, nil, err
	}

	upsert := &ContractUpsertInput{
		TenantID:             entity.TenantID,
		CapabilityKey:        entity.CapabilityKey,
		Version:              entity.Version,
		ProviderID:           entity.ProviderID,
		DisplayName:          entity.DisplayName,
		Description:          entity.Description,
		SecurityScope:        entity.SecurityScope,
		ToolGrantRequired:    entity.ToolGrantRequired,
		ObservabilityConfig:  fromJSONMap(entity.ObservabilityConfig),
		IOSchemas:            toSvcIOSchemas(entity.IOSchemas),
		TransportPreferences: fromJSONTransportPrefs(entity.TransportPreferences),
		ErrorTaxonomy:        toSvcErrorTaxonomy(entity.ErrorBindings),
	}
	profiles, err := s.transportRepo.ListByContract(ctx, entity.ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, err
	}
	upsert.TransportProfiles = toSvcTransportProfiles(profiles)

	issues, err := s.validate(ctx, upsert)
	if err != nil {
		return nil, issues, err
	}

	entity.LifecycleState = "published"
	entity.EffectiveAt = &in.EffectiveAt

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(entity).Error; err != nil {
			return err
		}
		return s.emitAuditAndEvent(ctx, tx, entity, "integration.capability.published", map[string]any{
			"notes": in.Notes,
		})
	})
	if err != nil {
		return nil, issues, err
	}

	res, buildErr := s.buildContract(ctx, entity)
	if buildErr != nil {
		return nil, issues, buildErr
	}
	return res, issues, nil
}

// DeprecateContract 将契约标记为废弃。
func (s *ContractService) DeprecateContract(ctx context.Context, in *DeprecateInput) (*Contract, error) {
	entity, err := s.contractRepo.FindByKeyVersion(ctx, in.TenantID, in.CapabilityKey, in.Version, true)
	if err != nil {
		return nil, err
	}
	entity.LifecycleState = "deprecated"
	entity.DeprecatedAt = &in.DeprecatedAt
	entity.ReplacementCapability = in.ReplacementCapability

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(entity).Error; err != nil {
			return err
		}
		return s.emitAuditAndEvent(ctx, tx, entity, "integration.capability.deprecated", map[string]any{
			"replacement": in.ReplacementCapability,
			"advisory":    in.AdvisoryMessage,
		})
	})
	if err != nil {
		return nil, err
	}
	return s.buildContract(ctx, entity)
}

func (s *ContractService) validate(ctx context.Context, in *ContractUpsertInput) ([]capability.ValidationIssue, error) {
	if s.validator == nil {
		return nil, nil
	}
	draft := &capability.CapabilityContractDraft{
		TenantID:             in.TenantID,
		CapabilityKey:        in.CapabilityKey,
		Version:              in.Version,
		ProviderID:           in.ProviderID,
		DisplayName:          in.DisplayName,
		SecurityScope:        in.SecurityScope,
		ToolGrantRequired:    in.ToolGrantRequired,
		IOSchemas:            in.IOSchemas,
		TransportPreferences: in.TransportPreferences,
		TransportProfiles:    in.TransportProfiles,
		ErrorTaxonomy:        in.ErrorTaxonomy,
	}
	issues, err := s.validator.ValidateContractDraft(ctx, draft)
	if err != nil {
		return issues, err
	}
	for _, issue := range issues {
		if issue.Severity == capability.SeverityFatal || issue.Severity == capability.SeverityError {
			return issues, ErrValidation
		}
	}
	return issues, nil
}

func (s *ContractService) buildContract(ctx context.Context, entity *capmodel.CapabilityContract) (*Contract, error) {
	if entity == nil {
		return nil, nil
	}
	prefs := fromJSONTransportPrefs(entity.TransportPreferences)
	ios := toSvcIOSchemas(entity.IOSchemas)
	bindings := toSvcErrorTaxonomy(entity.ErrorBindings)

	profiles, err := s.transportRepo.ListByContract(ctx, entity.ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	observability := fromJSONMap(entity.ObservabilityConfig)

	return &Contract{
		ID:                    entity.ID,
		ContractUUID:          entity.ContractUUID.String(),
		TenantID:              entity.TenantID,
		CapabilityKey:         entity.CapabilityKey,
		Version:               entity.Version,
		ProviderID:            entity.ProviderID,
		DisplayName:           entity.DisplayName,
		Description:           entity.Description,
		LifecycleState:        entity.LifecycleState,
		SecurityScope:         entity.SecurityScope,
		ToolGrantRequired:     entity.ToolGrantRequired,
		ObservabilityConfig:   observability,
		TransportPreferences:  prefs,
		IOSchemas:             ios,
		ErrorTaxonomy:         bindings,
		TransportProfiles:     toSvcTransportProfiles(profiles),
		EffectiveAt:           entity.EffectiveAt,
		DeprecatedAt:          entity.DeprecatedAt,
		ReplacementCapability: entity.ReplacementCapability,
		CreatedAt:             entity.CreatedAt,
		UpdatedAt:             entity.UpdatedAt,
	}, nil
}

func (s *ContractService) ensureErrorTaxonomy(ctx context.Context, tx *gorm.DB, tenantID uint64, contractID uint64, items []capability.ErrorTaxonomyEntry) ([]*capmodel.CapabilityContractErrorTaxonomy, error) {
	result := make([]*capmodel.CapabilityContractErrorTaxonomy, 0, len(items))
	for _, entry := range items {
		var taxonomy capmodel.CapabilityErrorTaxonomy
		err := tx.WithContext(ctx).
			Where("tenant_id = ? AND namespace = ? AND category = ? AND code = ?", tenantID, entry.Namespace, entry.Category, entry.Code).
			First(&taxonomy).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			severity := strings.ToUpper(entry.Severity)
			if severity == "" {
				severity = "ERROR"
			}
			stage := strings.ToLower(entry.Stage)
			taxonomy = capmodel.CapabilityErrorTaxonomy{
				TenantID:  tenantID,
				Namespace: entry.Namespace,
				Category:  entry.Category,
				Code:      entry.Code,
				Severity:  severity,
				Stage:     stage,
				Status:    1,
			}
			if err := tx.WithContext(ctx).Create(&taxonomy).Error; err != nil {
				logger.ErrorF(ctx, "[capability] create error taxonomy failed: %v", err)
				return nil, err
			}
		} else if err != nil {
			logger.ErrorF(ctx, "[capability] query error taxonomy failed: %v", err)
			return nil, err
		}
		result = append(result, &capmodel.CapabilityContractErrorTaxonomy{
			TenantID:        tenantID,
			ContractID:      contractID,
			ErrorTaxonomyID: taxonomy.ID,
		})
	}
	return result, nil
}

func (s *ContractService) emitAuditAndEvent(ctx context.Context, tx *gorm.DB, entity *capmodel.CapabilityContract, eventName string, payload map[string]any) error {
	if s.audit != nil {
		meta := map[string]any{
			"capability_key": entity.CapabilityKey,
			"version":        entity.Version,
			"provider_id":    entity.ProviderID,
		}
		for k, v := range payload {
			meta[k] = v
		}
		data, _ := json.Marshal(meta)
		_ = s.audit.Emit(ctx, &dbmaudit.AuditEvent{
			OccurredAt:   time.Now(),
			TenantID:     entity.TenantID,
			Source:       "capability.service",
			Operation:    eventName,
			ResourceType: "capability.contract",
			ResourceID:   fmt.Sprintf("%s:%s", entity.CapabilityKey, entity.Version),
			Outcome:      "SUCCESS",
			Severity:     "INFO",
			Meta:         datatypes.JSON(data),
		})
	}
	event_bus.Publish(event_bus.Event{
		Name:    eventName,
		Payload: payload,
		Ctx:     ctx,
	})
	return nil
}

func toModelIOSchemas(tenantID, contractID uint64, items []capability.IOSchemaDescriptor) []*capmodel.CapabilityIOSchema {
	result := make([]*capmodel.CapabilityIOSchema, 0, len(items))
	for _, schema := range items {
		result = append(result, &capmodel.CapabilityIOSchema{
			TenantID:        tenantID,
			ContractID:      contractID,
			Direction:       strings.ToLower(schema.Direction),
			Format:          strings.ToLower(schema.Format),
			SchemaURI:       schema.SchemaURI,
			SchemaHash:      schema.SchemaHash,
			ValidationRules: toJSONObject(schema.ValidationRules),
			Status:          1,
		})
	}
	return result
}

func toModelTransportProfiles(tenantID, contractID uint64, key string, items []capability.TransportProfile) []*capmodel.CapabilityTransportProfile {
	result := make([]*capmodel.CapabilityTransportProfile, 0, len(items))
	for _, profile := range items {
		result = append(result, &capmodel.CapabilityTransportProfile{
			TenantID:         tenantID,
			ContractID:       contractID,
			CapabilityKey:    key,
			Transport:        strings.ToLower(profile.Transport),
			Mode:             strings.ToLower(profile.Mode),
			TimeoutMillis:    profile.TimeoutMillis,
			RetryPolicy:      toJSONObject(profile.Retry),
			Streaming:        profile.Streaming,
			QoS:              toJSONObject(profile.QoS),
			EndpointSelector: toJSONObject(profile.EndpointSelector),
			LastHealthStatus: datatypes.JSON([]byte(`{}`)),
			Status:           1,
		})
	}
	return result
}

func toJSON(v interface{}) datatypes.JSON {
	if v == nil {
		return datatypes.JSON([]byte("null"))
	}
	data, err := json.Marshal(v)
	if err != nil {
		return datatypes.JSON([]byte("null"))
	}
	return datatypes.JSON(data)
}

func toJSONObject(v map[string]interface{}) datatypes.JSON {
	if v == nil {
		return datatypes.JSON([]byte(`{}`))
	}
	return toJSON(v)
}

func fromJSONMap(data datatypes.JSON) map[string]interface{} {
	if len(data) == 0 {
		return map[string]interface{}{}
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]interface{}{}
	}
	return out
}

func fromJSONTransportPrefs(data datatypes.JSON) []capability.TransportPreference {
	if len(data) == 0 {
		return nil
	}
	var out []capability.TransportPreference
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func toSvcIOSchemas(items []*capmodel.CapabilityIOSchema) []capability.IOSchemaDescriptor {
	result := make([]capability.IOSchemaDescriptor, 0, len(items))
	for _, item := range items {
		result = append(result, capability.IOSchemaDescriptor{
			Direction:       item.Direction,
			Format:          item.Format,
			SchemaURI:       item.SchemaURI,
			SchemaHash:      item.SchemaHash,
			ValidationRules: fromJSONMap(item.ValidationRules),
		})
	}
	return result
}

func toSvcErrorTaxonomy(items []*capmodel.CapabilityContractErrorTaxonomy) []capability.ErrorTaxonomyEntry {
	result := make([]capability.ErrorTaxonomyEntry, 0, len(items))
	for _, binding := range items {
		if binding.ErrorTaxonomy != nil {
			result = append(result, capability.ErrorTaxonomyEntry{
				Namespace: binding.ErrorTaxonomy.Namespace,
				Category:  binding.ErrorTaxonomy.Category,
				Code:      binding.ErrorTaxonomy.Code,
				Severity:  binding.ErrorTaxonomy.Severity,
				Stage:     binding.ErrorTaxonomy.Stage,
			})
		}
	}
	return result
}

func toSvcTransportProfiles(items []capmodel.CapabilityTransportProfile) []capability.TransportProfile {
	result := make([]capability.TransportProfile, 0, len(items))
	for _, profile := range items {
		result = append(result, capability.TransportProfile{
			Transport:        profile.Transport,
			Mode:             profile.Mode,
			TimeoutMillis:    profile.TimeoutMillis,
			Streaming:        profile.Streaming,
			Retry:            fromJSONMap(profile.RetryPolicy),
			QoS:              fromJSONMap(profile.QoS),
			EndpointSelector: fromJSONMap(profile.EndpointSelector),
		})
	}
	return result
}
