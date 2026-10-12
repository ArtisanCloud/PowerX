package metadata

import (
	"context"
	"errors"
	"strings"

	metadto "github.com/ArtisanCloud/PowerX/internal/dto/metadata"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	metarepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/metadata"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ListTagBindingsInput struct {
	TenantUUID   string
	ResourceType string
	ResourceUUID string
	Locale       string
}

type ReplaceTagBindingsInput struct {
	TenantUUID    string
	ResourceType  string
	ResourceUUID  string
	TagUUIDs      []string
	CreatedByUUID string
	Locale        string
}

type CreateTagBindingInput struct {
	TenantUUID    string
	TagUUID       string
	ResourceType  string
	ResourceUUID  string
	CreatedByUUID string
}

func (s *Service) tagBindingRepo() *metarepo.TagBindingRepository {
	return metarepo.NewTagBindingRepository(s.deps.DB)
}

func (s *Service) ListTagBindings(ctx context.Context, in ListTagBindingsInput) ([]metadto.TagBindingResponse, error) {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return nil, err
	}
	resourceType := strings.TrimSpace(in.ResourceType)
	if err := ValidateMachineIdentifier(resourceType); err != nil {
		return nil, err
	}
	resourceUUID := strings.TrimSpace(in.ResourceUUID)
	if err := validResourceUUID(resourceUUID); err != nil {
		return nil, err
	}
	bindings, tags, err := s.tagBindingRepo().ListByResource(ctx, tenantUUID, resourceType, resourceUUID)
	if err != nil {
		return nil, err
	}
	return mapTagBindings(bindings, tags, localeOrDefault(in.Locale)), nil
}

func (s *Service) ReplaceTagBindings(ctx context.Context, in ReplaceTagBindingsInput) ([]metadto.TagBindingResponse, error) {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return nil, err
	}
	resourceType := strings.TrimSpace(in.ResourceType)
	if err := ValidateMachineIdentifier(resourceType); err != nil {
		return nil, err
	}
	resourceUUID := strings.TrimSpace(in.ResourceUUID)
	if err := validResourceUUID(resourceUUID); err != nil {
		return nil, err
	}
	if err := s.validateBindableResource(ctx, tenantUUID, resourceType, resourceUUID); err != nil {
		return nil, err
	}
	uniqueTagUUIDs := make([]string, 0, len(in.TagUUIDs))
	seen := map[string]struct{}{}
	for _, tagUUID := range in.TagUUIDs {
		tagUUID = strings.TrimSpace(tagUUID)
		if tagUUID == "" {
			return nil, ErrUUIDRequired
		}
		if _, ok := seen[tagUUID]; ok {
			continue
		}
		tag, err := s.tagRepo().GetTag(ctx, tenantUUID, tagUUID)
		if err != nil {
			return nil, err
		}
		if tag.ResourceType != resourceType {
			return nil, ErrTagResourceMismatch
		}
		if tag.Status != model.StatusEnabled {
			return nil, ErrTagDisabled
		}
		seen[tagUUID] = struct{}{}
		uniqueTagUUIDs = append(uniqueTagUUIDs, tagUUID)
	}
	if _, err := s.tagBindingRepo().ReplaceByResource(ctx, tenantUUID, resourceType, resourceUUID, strings.TrimSpace(in.CreatedByUUID), uniqueTagUUIDs); err != nil {
		return nil, err
	}
	s.publishAudit(ctx, AuditEvent{TenantUUID: tenantUUID, Operation: "replace", ObjectType: "tag_binding", ObjectUUID: resourceUUID})
	return s.ListTagBindings(ctx, ListTagBindingsInput{
		TenantUUID:   tenantUUID,
		ResourceType: resourceType,
		ResourceUUID: resourceUUID,
		Locale:       in.Locale,
	})
}

func (s *Service) CreateTagBinding(ctx context.Context, in CreateTagBindingInput) (metadto.TagBindingResponse, error) {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return metadto.TagBindingResponse{}, err
	}
	if err = validResourceUUID(strings.TrimSpace(in.TagUUID)); err != nil {
		return metadto.TagBindingResponse{}, err
	}
	resourceType := strings.TrimSpace(in.ResourceType)
	if err = ValidateMachineIdentifier(resourceType); err != nil {
		return metadto.TagBindingResponse{}, err
	}
	resourceUUID := strings.TrimSpace(in.ResourceUUID)
	if err = validResourceUUID(resourceUUID); err != nil {
		return metadto.TagBindingResponse{}, err
	}
	if err = s.validateBindableResource(ctx, tenantUUID, resourceType, resourceUUID); err != nil {
		return metadto.TagBindingResponse{}, err
	}
	tag, err := s.tagRepo().GetTag(ctx, tenantUUID, strings.TrimSpace(in.TagUUID))
	if err != nil {
		return metadto.TagBindingResponse{}, err
	}
	if tag.ResourceType != resourceType || tag.Status != model.StatusEnabled {
		return metadto.TagBindingResponse{}, ErrTagResourceMismatch
	}
	binding := &model.TagBinding{BindingUUID: uuid.NewString(), TenantUUID: tenantUUID, TagUUID: tag.UUID.String(), ResourceType: resourceType, ResourceUUID: resourceUUID, CreatedByUUID: strings.TrimSpace(in.CreatedByUUID)}
	if err = s.tagBindingRepo().Create(ctx, binding); err != nil {
		return metadto.TagBindingResponse{}, err
	}
	s.publishAudit(ctx, AuditEvent{TenantUUID: tenantUUID, Operation: "create", ObjectType: "tag_binding", ObjectUUID: binding.BindingUUID})
	mapped := mapTag(tag, "zh-CN")
	return metadto.TagBindingResponse{BindingUUID: binding.BindingUUID, TagUUID: binding.TagUUID, ResourceType: binding.ResourceType, ResourceUUID: binding.ResourceUUID, Tag: &mapped}, nil
}

func (s *Service) DeleteTagBinding(ctx context.Context, tenantUUID, bindingUUID string) error {
	tenantUUID, err := canonicalTenant(tenantUUID)
	if err != nil {
		return err
	}
	if err = validResourceUUID(strings.TrimSpace(bindingUUID)); err != nil {
		return err
	}
	binding, err := s.tagBindingRepo().DeleteByBindingUUID(ctx, tenantUUID, strings.TrimSpace(bindingUUID))
	if err != nil {
		return err
	}
	var total int64
	if err = s.deps.DB.WithContext(ctx).Model(&model.TagBinding{}).Where("tenant_uuid = ? AND tag_uuid = ?", tenantUUID, binding.TagUUID).Count(&total).Error; err != nil {
		return err
	}
	if err = s.deps.DB.WithContext(ctx).Model(&model.Tag{}).Where("tenant_uuid = ? AND uuid = ?", tenantUUID, binding.TagUUID).Update("usage_count", total).Error; err != nil {
		return err
	}
	s.publishAudit(ctx, AuditEvent{TenantUUID: tenantUUID, Operation: "delete", ObjectType: "tag_binding", ObjectUUID: bindingUUID})
	return nil
}

func (s *Service) validateBindableResource(ctx context.Context, tenantUUID, resourceType, resourceUUID string) error {
	row, err := s.resourceTypeRepo().GetByResourceType(ctx, tenantUUID, resourceType)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrResourceTypeMissing
	}
	if err != nil {
		return err
	}
	if row.Status != model.StatusEnabled || !row.BindingEnabled {
		return ErrResourceBindingDisabled
	}
	validatorKey := strings.TrimSpace(row.ValidatorKey)
	if validatorKey == "" {
		return ErrResourceValidatorMissing
	}
	validator, ok := s.deps.ValidatorRegistry.Get(validatorKey)
	if !ok || validator == nil {
		return ErrResourceValidatorMissing
	}
	return validator.ValidateResource(ctx, tenantUUID, resourceUUID)
}

func mapTagBindings(bindings []model.TagBinding, tags []model.Tag, locale string) []metadto.TagBindingResponse {
	tagByUUID := make(map[string]*metadto.TagResponse, len(tags))
	for i := range tags {
		mapped := mapTag(&tags[i], locale)
		tagByUUID[mapped.UUID] = &mapped
	}
	out := make([]metadto.TagBindingResponse, 0, len(bindings))
	for i := range bindings {
		b := bindings[i]
		out = append(out, metadto.TagBindingResponse{
			BindingUUID:  b.BindingUUID,
			TagUUID:      b.TagUUID,
			ResourceType: b.ResourceType,
			ResourceUUID: b.ResourceUUID,
			Tag:          tagByUUID[b.TagUUID],
		})
	}
	return out
}
