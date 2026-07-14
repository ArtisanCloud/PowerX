package metadata

import (
	"context"
	"errors"
	"strings"

	metadto "github.com/ArtisanCloud/PowerX/internal/dto/metadata"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	metarepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/metadata"
	"github.com/google/uuid"
)

var (
	ErrTagBound            = errors.New("metadata.tag_bound")
	ErrTagResourceMismatch = errors.New("metadata.tag_resource_mismatch")
	ErrTagDisabled         = errors.New("metadata.tag_disabled")
	ErrMergeSameTag        = errors.New("metadata.merge_same_tag")
)

type CreateTagInput struct {
	TenantUUID      string
	Namespace       string
	ResourceType    string
	Code            string
	LabelI18n       map[string]string
	DescriptionI18n map[string]string
	Color           string
}

type UpdateTagInput struct {
	TenantUUID      string
	TagUUID         string
	LabelI18n       *map[string]string
	DescriptionI18n *map[string]string
	Color           *string
	Status          *string
}

type ListTagsInput struct {
	TenantUUID   string
	Namespace    string
	ResourceType string
	Status       string
	Query        string
	Locale       string
	Page         int
	PageSize     int
}

type DeleteTagInput struct {
	TenantUUID string
	TagUUID    string
}

type MergeTagsInput struct {
	TenantUUID    string
	SourceTagUUID string
	TargetTagUUID string
}

type TagPage struct {
	Items    []metadto.TagResponse
	Total    int64
	Page     int
	PageSize int
}

func (s *Service) tagRepo() *metarepo.TagRepository {
	return metarepo.NewTagRepository(s.deps.DB)
}

func (s *Service) CreateTag(ctx context.Context, in CreateTagInput) (metadto.TagResponse, error) {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return metadto.TagResponse{}, err
	}
	namespace := strings.TrimSpace(in.Namespace)
	if err := ValidateMachineIdentifier(namespace); err != nil {
		return metadto.TagResponse{}, err
	}
	resourceType := strings.TrimSpace(in.ResourceType)
	if err := ValidateMachineIdentifier(resourceType); err != nil {
		return metadto.TagResponse{}, err
	}
	code := strings.TrimSpace(in.Code)
	if err := ValidateMachineIdentifier(code); err != nil {
		return metadto.TagResponse{}, err
	}
	if err := ValidateRequiredI18n(in.LabelI18n, "zh-CN"); err != nil {
		return metadto.TagResponse{}, err
	}
	row := &model.Tag{
		TenantUUID:      tenantUUID,
		Namespace:       namespace,
		ResourceType:    resourceType,
		Code:            code,
		LabelI18n:       mustJSON(in.LabelI18n),
		DescriptionI18n: mustJSON(in.DescriptionI18n),
		Color:           strings.TrimSpace(in.Color),
		Source:          model.SourceAdmin,
		Status:          model.StatusEnabled,
	}
	if err := s.tagRepo().CreateTag(ctx, row); err != nil {
		return metadto.TagResponse{}, err
	}
	s.publishAudit(ctx, AuditEvent{TenantUUID: tenantUUID, Operation: "create", ObjectType: "tag", ObjectUUID: row.UUID.String()})
	return mapTag(row, "zh-CN"), nil
}

func (s *Service) UpdateTag(ctx context.Context, in UpdateTagInput) (metadto.TagResponse, error) {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return metadto.TagResponse{}, err
	}
	tagUUID := strings.TrimSpace(in.TagUUID)
	if tagUUID == "" {
		return metadto.TagResponse{}, ErrUUIDRequired
	}
	updates := map[string]any{}
	if in.LabelI18n != nil {
		if err := ValidateRequiredI18n(*in.LabelI18n, "zh-CN"); err != nil {
			return metadto.TagResponse{}, err
		}
		updates["label_i18n"] = mustJSON(*in.LabelI18n)
	}
	if in.DescriptionI18n != nil {
		updates["description_i18n"] = mustJSON(*in.DescriptionI18n)
	}
	if in.Color != nil {
		updates["color"] = strings.TrimSpace(*in.Color)
	}
	if in.Status != nil {
		status := strings.TrimSpace(*in.Status)
		if err := ValidateStatus(status); err != nil {
			return metadto.TagResponse{}, err
		}
		updates["status"] = status
	}
	row, err := s.tagRepo().UpdateTag(ctx, tenantUUID, tagUUID, updates)
	if err != nil {
		return metadto.TagResponse{}, err
	}
	s.publishAudit(ctx, AuditEvent{TenantUUID: tenantUUID, Operation: "update", ObjectType: "tag", ObjectUUID: row.UUID.String()})
	return mapTag(row, "zh-CN"), nil
}

func (s *Service) ListTags(ctx context.Context, in ListTagsInput) (TagPage, error) {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return TagPage{}, err
	}
	if strings.TrimSpace(in.Status) != "" {
		if err := ValidateStatus(strings.TrimSpace(in.Status)); err != nil {
			return TagPage{}, err
		}
	}
	rows, total, err := s.tagRepo().ListTags(ctx, metarepo.TagListOptions{
		TenantUUID:   tenantUUID,
		Namespace:    in.Namespace,
		ResourceType: in.ResourceType,
		Status:       in.Status,
		Query:        in.Query,
		Page:         normalizedPage(in.Page),
		PageSize:     normalizedPageSize(in.PageSize),
	})
	if err != nil {
		return TagPage{}, err
	}
	items := make([]metadto.TagResponse, 0, len(rows))
	for i := range rows {
		items = append(items, mapTag(&rows[i], localeOrDefault(in.Locale)))
	}
	return TagPage{Items: items, Total: total, Page: normalizedPage(in.Page), PageSize: normalizedPageSize(in.PageSize)}, nil
}

func (s *Service) DeleteTag(ctx context.Context, in DeleteTagInput) error {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return err
	}
	tagUUID := strings.TrimSpace(in.TagUUID)
	if tagUUID == "" {
		return ErrUUIDRequired
	}
	total, err := s.tagRepo().CountBindings(ctx, tenantUUID, tagUUID)
	if err != nil {
		return err
	}
	if total > 0 {
		return ErrTagBound
	}
	if err := s.tagRepo().DeleteTag(ctx, tenantUUID, tagUUID); err != nil {
		return err
	}
	s.publishAudit(ctx, AuditEvent{TenantUUID: tenantUUID, Operation: "delete", ObjectType: "tag", ObjectUUID: tagUUID})
	return nil
}

func (s *Service) MergeTags(ctx context.Context, in MergeTagsInput) (int64, error) {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return 0, err
	}
	sourceUUID := strings.TrimSpace(in.SourceTagUUID)
	targetUUID := strings.TrimSpace(in.TargetTagUUID)
	if sourceUUID == "" || targetUUID == "" {
		return 0, ErrUUIDRequired
	}
	if sourceUUID == targetUUID {
		return 0, ErrMergeSameTag
	}
	source, err := s.tagRepo().GetTag(ctx, tenantUUID, sourceUUID)
	if err != nil {
		return 0, err
	}
	target, err := s.tagRepo().GetTag(ctx, tenantUUID, targetUUID)
	if err != nil {
		return 0, err
	}
	if source.ResourceType != target.ResourceType {
		return 0, ErrTagResourceMismatch
	}
	moved, err := s.tagRepo().MergeTags(ctx, tenantUUID, sourceUUID, targetUUID)
	if err != nil {
		return 0, err
	}
	s.publishAudit(ctx, AuditEvent{TenantUUID: tenantUUID, Operation: "merge", ObjectType: "tag", ObjectUUID: targetUUID})
	return moved, nil
}

func mapTag(row *model.Tag, locale string) metadto.TagResponse {
	label := mapStringJSON(row.LabelI18n)
	desc := mapStringJSON(row.DescriptionI18n)
	displayName, missing := localized(label, locale)
	displayDesc, _ := localized(desc, locale)
	return metadto.TagResponse{
		UUID:            row.UUID.String(),
		Namespace:       row.Namespace,
		ResourceType:    row.ResourceType,
		Code:            row.Code,
		LabelI18n:       label,
		DescriptionI18n: desc,
		Color:           row.Color,
		Status:          row.Status,
		UsageCount:      row.UsageCount,
		Display: metadto.Display{
			DisplayName:          displayName,
			DisplayDescription:   displayDesc,
			DisplayLocale:        locale,
			DisplayLocaleMissing: missing,
		},
	}
}

func validResourceUUID(value string) error {
	if _, err := uuid.Parse(strings.TrimSpace(value)); err != nil {
		return ErrUUIDRequired
	}
	return nil
}
