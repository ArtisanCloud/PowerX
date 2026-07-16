package metadata

import (
	"context"
	"errors"
	"strings"

	metadto "github.com/ArtisanCloud/PowerX/internal/dto/metadata"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	metarepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/metadata"
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
			TagUUID:      b.TagUUID,
			ResourceType: b.ResourceType,
			ResourceUUID: b.ResourceUUID,
			Tag:          tagByUUID[b.TagUUID],
		})
	}
	return out
}
