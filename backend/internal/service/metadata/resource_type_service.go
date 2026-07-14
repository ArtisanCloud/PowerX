package metadata

import (
	"context"
	"errors"
	"strings"

	metadto "github.com/ArtisanCloud/PowerX/internal/dto/metadata"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	metarepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/metadata"
)

const (
	ValidatorStatusAvailable = "available"
	ValidatorStatusMissing   = "missing"
	ValidatorStatusDisabled  = "disabled"
)

var (
	ErrResourceTypeMissing      = errors.New("metadata.resource_type_missing")
	ErrResourceBindingDisabled  = errors.New("metadata.resource_binding_disabled")
	ErrResourceValidatorMissing = errors.New("metadata.resource_validator_missing")
)

type RegisterResourceTypeInput struct {
	TenantUUID      string
	ResourceType    string
	Module          string
	NameI18n        map[string]string
	DescriptionI18n map[string]string
	ValidatorKey    string
	BindingEnabled  bool
}

type UpdateResourceTypeInput struct {
	TenantUUID       string
	ResourceTypeUUID string
	NameI18n         *map[string]string
	DescriptionI18n  *map[string]string
	ValidatorKey     *string
	BindingEnabled   *bool
	Status           *string
}

type ListResourceTypesInput struct {
	TenantUUID string
	Module     string
	Status     string
	Query      string
	Locale     string
	Page       int
	PageSize   int
}

type ResourceTypePage struct {
	Items    []metadto.ResourceTypeResponse
	Total    int64
	Page     int
	PageSize int
}

func (s *Service) resourceTypeRepo() *metarepo.ResourceTypeRepository {
	return metarepo.NewResourceTypeRepository(s.deps.DB)
}

func (s *Service) RegisterResourceType(ctx context.Context, in RegisterResourceTypeInput) (metadto.ResourceTypeResponse, error) {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return metadto.ResourceTypeResponse{}, err
	}
	resourceType := strings.TrimSpace(in.ResourceType)
	if err := ValidateMachineIdentifier(resourceType); err != nil {
		return metadto.ResourceTypeResponse{}, err
	}
	module := strings.TrimSpace(in.Module)
	if err := ValidateMachineIdentifier(module); err != nil {
		return metadto.ResourceTypeResponse{}, err
	}
	if err := ValidateRequiredI18n(in.NameI18n, "zh-CN"); err != nil {
		return metadto.ResourceTypeResponse{}, err
	}
	validatorKey := strings.TrimSpace(in.ValidatorKey)
	if in.BindingEnabled && validatorKey == "" {
		return metadto.ResourceTypeResponse{}, ErrResourceValidatorMissing
	}
	row := &model.ResourceType{
		TenantUUID:      tenantUUID,
		ResourceType:    resourceType,
		Module:          module,
		NameI18n:        mustJSON(in.NameI18n),
		DescriptionI18n: mustJSON(in.DescriptionI18n),
		ValidatorKey:    validatorKey,
		BindingEnabled:  in.BindingEnabled,
		Status:          model.StatusEnabled,
	}
	if err := s.resourceTypeRepo().Create(ctx, row); err != nil {
		return metadto.ResourceTypeResponse{}, err
	}
	s.publishAudit(ctx, AuditEvent{TenantUUID: tenantUUID, Operation: "register", ObjectType: "resource_type", ObjectUUID: row.UUID.String()})
	return s.mapResourceType(row, "zh-CN"), nil
}

func (s *Service) UpdateResourceType(ctx context.Context, in UpdateResourceTypeInput) (metadto.ResourceTypeResponse, error) {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return metadto.ResourceTypeResponse{}, err
	}
	resourceTypeUUID := strings.TrimSpace(in.ResourceTypeUUID)
	if resourceTypeUUID == "" {
		return metadto.ResourceTypeResponse{}, ErrUUIDRequired
	}
	updates := map[string]any{}
	if in.NameI18n != nil {
		if err := ValidateRequiredI18n(*in.NameI18n, "zh-CN"); err != nil {
			return metadto.ResourceTypeResponse{}, err
		}
		updates["name_i18n"] = mustJSON(*in.NameI18n)
	}
	if in.DescriptionI18n != nil {
		updates["description_i18n"] = mustJSON(*in.DescriptionI18n)
	}
	if in.ValidatorKey != nil {
		updates["validator_key"] = strings.TrimSpace(*in.ValidatorKey)
	}
	if in.BindingEnabled != nil {
		updates["binding_enabled"] = *in.BindingEnabled
	}
	if in.Status != nil {
		status := strings.TrimSpace(*in.Status)
		if err := ValidateStatus(status); err != nil {
			return metadto.ResourceTypeResponse{}, err
		}
		updates["status"] = status
	}
	current, err := s.resourceTypeRepo().Get(ctx, tenantUUID, resourceTypeUUID)
	if err != nil {
		return metadto.ResourceTypeResponse{}, err
	}
	finalBindingEnabled := current.BindingEnabled
	if in.BindingEnabled != nil {
		finalBindingEnabled = *in.BindingEnabled
	}
	finalValidatorKey := strings.TrimSpace(current.ValidatorKey)
	if in.ValidatorKey != nil {
		finalValidatorKey = strings.TrimSpace(*in.ValidatorKey)
	}
	if finalBindingEnabled && finalValidatorKey == "" {
		return metadto.ResourceTypeResponse{}, ErrResourceValidatorMissing
	}
	row, err := s.resourceTypeRepo().Update(ctx, tenantUUID, resourceTypeUUID, updates)
	if err != nil {
		return metadto.ResourceTypeResponse{}, err
	}
	s.publishAudit(ctx, AuditEvent{TenantUUID: tenantUUID, Operation: "update", ObjectType: "resource_type", ObjectUUID: row.UUID.String()})
	return s.mapResourceType(row, "zh-CN"), nil
}

func (s *Service) ListResourceTypes(ctx context.Context, in ListResourceTypesInput) (ResourceTypePage, error) {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return ResourceTypePage{}, err
	}
	if strings.TrimSpace(in.Status) != "" {
		if err := ValidateStatus(strings.TrimSpace(in.Status)); err != nil {
			return ResourceTypePage{}, err
		}
	}
	rows, total, err := s.resourceTypeRepo().List(ctx, metarepo.ResourceTypeListOptions{
		TenantUUID: tenantUUID,
		Module:     in.Module,
		Status:     in.Status,
		Query:      in.Query,
		Page:       normalizedPage(in.Page),
		PageSize:   normalizedPageSize(in.PageSize),
	})
	if err != nil {
		return ResourceTypePage{}, err
	}
	items := make([]metadto.ResourceTypeResponse, 0, len(rows))
	for i := range rows {
		items = append(items, s.mapResourceType(&rows[i], localeOrDefault(in.Locale)))
	}
	return ResourceTypePage{Items: items, Total: total, Page: normalizedPage(in.Page), PageSize: normalizedPageSize(in.PageSize)}, nil
}

func (s *Service) mapResourceType(row *model.ResourceType, locale string) metadto.ResourceTypeResponse {
	name := mapStringJSON(row.NameI18n)
	desc := mapStringJSON(row.DescriptionI18n)
	displayName, missing := localized(name, locale)
	displayDesc, _ := localized(desc, locale)
	return metadto.ResourceTypeResponse{
		UUID:            row.UUID.String(),
		ResourceType:    row.ResourceType,
		Module:          row.Module,
		NameI18n:        name,
		DescriptionI18n: desc,
		ValidatorKey:    row.ValidatorKey,
		BindingEnabled:  row.BindingEnabled,
		ValidatorStatus: s.validatorStatus(row),
		Status:          row.Status,
		Display: metadto.Display{
			DisplayName:          displayName,
			DisplayDescription:   displayDesc,
			DisplayLocale:        locale,
			DisplayLocaleMissing: missing,
		},
	}
}

func (s *Service) validatorStatus(row *model.ResourceType) string {
	if row == nil || !row.BindingEnabled || row.Status != model.StatusEnabled {
		return ValidatorStatusDisabled
	}
	if strings.TrimSpace(row.ValidatorKey) == "" {
		return ValidatorStatusMissing
	}
	if _, ok := s.deps.ValidatorRegistry.Get(strings.TrimSpace(row.ValidatorKey)); ok {
		return ValidatorStatusAvailable
	}
	return ValidatorStatusMissing
}
