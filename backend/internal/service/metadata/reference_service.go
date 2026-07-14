package metadata

import (
	"context"
	"errors"
	"strings"

	metadto "github.com/ArtisanCloud/PowerX/internal/dto/metadata"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	metarepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/metadata"
)

var ErrReferenceResourceMismatch = errors.New("metadata.reference_resource_mismatch")

type MetadataReferenceInput struct {
	MetadataType string
	MetadataUUID string
	ResourceType string
	ResourceUUID string
	FieldName    string
}

type RegisterMetadataReferencesInput struct {
	TenantUUID string
	References []MetadataReferenceInput
}

type ReplaceMetadataReferencesInput struct {
	TenantUUID   string
	ResourceType string
	ResourceUUID string
	References   []MetadataReferenceInput
}

type DeleteMetadataReferencesInput struct {
	TenantUUID   string
	ResourceType string
	ResourceUUID string
}

func (s *Service) referenceRepo() *metarepo.ReferenceRepository {
	return metarepo.NewReferenceRepository(s.deps.DB)
}

func (s *Service) RegisterMetadataReferences(ctx context.Context, in RegisterMetadataReferencesInput) ([]metadto.MetadataReferenceResponse, error) {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return nil, err
	}
	rows, err := s.normalizeReferences(tenantUUID, in.References)
	if err != nil {
		return nil, err
	}
	if err := s.referenceRepo().Register(ctx, rows); err != nil {
		return nil, err
	}
	s.publishAudit(ctx, AuditEvent{TenantUUID: tenantUUID, Operation: "register", ObjectType: "metadata_reference", ObjectUUID: ""})
	return mapMetadataReferences(rows), nil
}

func (s *Service) ReplaceMetadataReferencesForResource(ctx context.Context, in ReplaceMetadataReferencesInput) ([]metadto.MetadataReferenceResponse, error) {
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
	rows, err := s.normalizeReferences(tenantUUID, in.References)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].ResourceType != resourceType || rows[i].ResourceUUID != resourceUUID {
			return nil, ErrReferenceResourceMismatch
		}
	}
	if err := s.referenceRepo().ReplaceForResource(ctx, tenantUUID, resourceType, resourceUUID, rows); err != nil {
		return nil, err
	}
	s.publishAudit(ctx, AuditEvent{TenantUUID: tenantUUID, Operation: "replace", ObjectType: "metadata_reference", ObjectUUID: resourceUUID})
	return mapMetadataReferences(rows), nil
}

func (s *Service) DeleteMetadataReferencesForResource(ctx context.Context, in DeleteMetadataReferencesInput) error {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return err
	}
	resourceType := strings.TrimSpace(in.ResourceType)
	if err := ValidateMachineIdentifier(resourceType); err != nil {
		return err
	}
	resourceUUID := strings.TrimSpace(in.ResourceUUID)
	if err := validResourceUUID(resourceUUID); err != nil {
		return err
	}
	if err := s.referenceRepo().DeleteForResource(ctx, tenantUUID, resourceType, resourceUUID); err != nil {
		return err
	}
	s.publishAudit(ctx, AuditEvent{TenantUUID: tenantUUID, Operation: "delete", ObjectType: "metadata_reference", ObjectUUID: resourceUUID})
	return nil
}

func (s *Service) normalizeReferences(tenantUUID string, refs []MetadataReferenceInput) ([]model.Reference, error) {
	rows := make([]model.Reference, 0, len(refs))
	for i := range refs {
		ref := refs[i]
		metadataType := strings.TrimSpace(ref.MetadataType)
		if err := ValidateMachineIdentifier(metadataType); err != nil {
			return nil, err
		}
		metadataUUID := strings.TrimSpace(ref.MetadataUUID)
		if err := validResourceUUID(metadataUUID); err != nil {
			return nil, err
		}
		resourceType := strings.TrimSpace(ref.ResourceType)
		if err := ValidateMachineIdentifier(resourceType); err != nil {
			return nil, err
		}
		resourceUUID := strings.TrimSpace(ref.ResourceUUID)
		if err := validResourceUUID(resourceUUID); err != nil {
			return nil, err
		}
		fieldName := strings.TrimSpace(ref.FieldName)
		if err := ValidateMachineIdentifier(fieldName); err != nil {
			return nil, err
		}
		rows = append(rows, model.Reference{
			TenantUUID:   tenantUUID,
			MetadataType: metadataType,
			MetadataUUID: metadataUUID,
			ResourceType: resourceType,
			ResourceUUID: resourceUUID,
			FieldName:    fieldName,
		})
	}
	return rows, nil
}

func mapMetadataReferences(rows []model.Reference) []metadto.MetadataReferenceResponse {
	out := make([]metadto.MetadataReferenceResponse, 0, len(rows))
	for i := range rows {
		out = append(out, metadto.MetadataReferenceResponse{
			MetadataType: rows[i].MetadataType,
			MetadataUUID: rows[i].MetadataUUID,
			ResourceType: rows[i].ResourceType,
			ResourceUUID: rows[i].ResourceUUID,
			FieldName:    rows[i].FieldName,
		})
	}
	return out
}
