package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	metadto "github.com/ArtisanCloud/PowerX/internal/dto/metadata"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	metarepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/metadata"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var (
	ErrReferenceConflict = errors.New("metadata.reference_conflict")
	ErrUUIDRequired      = errors.New("metadata.uuid_required")
)

type CreateDictionaryNamespaceInput struct {
	TenantUUID      string
	Namespace       string
	Module          string
	NameI18n        map[string]string
	DescriptionI18n map[string]string
}

type UpdateDictionaryNamespaceInput struct {
	TenantUUID      string
	NamespaceUUID   string
	NameI18n        *map[string]string
	DescriptionI18n *map[string]string
	Status          *string
}

type ListDictionaryNamespacesInput struct {
	TenantUUID string
	Module     string
	Status     string
	Query      string
	Locale     string
	Page       int
	PageSize   int
}

type CreateDictionaryItemInput struct {
	TenantUUID      string
	NamespaceUUID   string
	Code            string
	LabelI18n       map[string]string
	DescriptionI18n map[string]string
	SortOrder       int
	Metadata        map[string]any
}

type UpdateDictionaryItemInput struct {
	TenantUUID      string
	ItemUUID        string
	LabelI18n       *map[string]string
	DescriptionI18n *map[string]string
	SortOrder       *int
	Status          *string
	Metadata        *map[string]any
}

type ListDictionaryItemsInput struct {
	TenantUUID    string
	NamespaceUUID string
	Status        string
	Query         string
	Locale        string
	Page          int
	PageSize      int
}

type DeleteDictionaryItemInput struct {
	TenantUUID string
	ItemUUID   string
}

type DictionaryNamespacePage struct {
	Items    []metadto.DictionaryNamespaceResponse
	Total    int64
	Page     int
	PageSize int
}

type DictionaryItemPage struct {
	Items    []metadto.DictionaryItemResponse
	Total    int64
	Page     int
	PageSize int
}

func (s *Service) dictionaryRepo() *metarepo.DictionaryRepository {
	return metarepo.NewDictionaryRepository(s.deps.DB)
}

func (s *Service) CreateDictionaryNamespace(ctx context.Context, in CreateDictionaryNamespaceInput) (metadto.DictionaryNamespaceResponse, error) {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return metadto.DictionaryNamespaceResponse{}, err
	}
	namespace := strings.TrimSpace(in.Namespace)
	if err := ValidateMachineIdentifier(namespace); err != nil {
		return metadto.DictionaryNamespaceResponse{}, err
	}
	module := strings.TrimSpace(in.Module)
	if err := ValidateNamespaceInModule(namespace, module); err != nil {
		return metadto.DictionaryNamespaceResponse{}, err
	}
	if err := ValidateRequiredI18n(in.NameI18n, "zh-CN"); err != nil {
		return metadto.DictionaryNamespaceResponse{}, err
	}
	if _, err := s.dictionaryRepo().GetNamespaceByNamespace(ctx, tenantUUID, namespace); err == nil {
		return metadto.DictionaryNamespaceResponse{}, ErrAlreadyExists
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return metadto.DictionaryNamespaceResponse{}, err
	}
	row := &model.DictionaryNamespace{
		TenantUUID:      tenantUUID,
		Namespace:       namespace,
		Module:          module,
		NameI18n:        mustJSON(in.NameI18n),
		DescriptionI18n: mustJSON(in.DescriptionI18n),
		Status:          model.StatusEnabled,
	}
	if err := s.dictionaryRepo().CreateNamespace(ctx, row); err != nil {
		return metadto.DictionaryNamespaceResponse{}, err
	}
	s.publishAudit(ctx, AuditEvent{TenantUUID: tenantUUID, Operation: "create", ObjectType: "dictionary_namespace", ObjectUUID: row.UUID.String()})
	return mapNamespace(row, "zh-CN", 0), nil
}

func (s *Service) UpdateDictionaryNamespace(ctx context.Context, in UpdateDictionaryNamespaceInput) (metadto.DictionaryNamespaceResponse, error) {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return metadto.DictionaryNamespaceResponse{}, err
	}
	namespaceUUID := strings.TrimSpace(in.NamespaceUUID)
	if namespaceUUID == "" {
		return metadto.DictionaryNamespaceResponse{}, ErrUUIDRequired
	}
	updates := map[string]any{}
	if in.NameI18n != nil {
		if err := ValidateRequiredI18n(*in.NameI18n, "zh-CN"); err != nil {
			return metadto.DictionaryNamespaceResponse{}, err
		}
		updates["name_i18n"] = mustJSON(*in.NameI18n)
	}
	if in.DescriptionI18n != nil {
		updates["description_i18n"] = mustJSON(*in.DescriptionI18n)
	}
	if in.Status != nil {
		status := strings.TrimSpace(*in.Status)
		if err := ValidateStatus(status); err != nil {
			return metadto.DictionaryNamespaceResponse{}, err
		}
		updates["status"] = status
	}
	row, err := s.dictionaryRepo().UpdateNamespace(ctx, tenantUUID, namespaceUUID, updates)
	if err != nil {
		return metadto.DictionaryNamespaceResponse{}, err
	}
	s.publishAudit(ctx, AuditEvent{TenantUUID: tenantUUID, Operation: "update", ObjectType: "dictionary_namespace", ObjectUUID: row.UUID.String()})
	return mapNamespace(row, "zh-CN", 0), nil
}

func (s *Service) ListDictionaryNamespaces(ctx context.Context, in ListDictionaryNamespacesInput) (DictionaryNamespacePage, error) {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return DictionaryNamespacePage{}, err
	}
	if strings.TrimSpace(in.Status) != "" {
		if err := ValidateStatus(strings.TrimSpace(in.Status)); err != nil {
			return DictionaryNamespacePage{}, err
		}
	}
	rows, total, err := s.dictionaryRepo().ListNamespaces(ctx, metarepo.DictionaryNamespaceListOptions{
		TenantUUID: tenantUUID,
		Module:     in.Module,
		Status:     in.Status,
		Query:      in.Query,
		Page:       normalizedPage(in.Page),
		PageSize:   normalizedPageSize(in.PageSize),
	})
	if err != nil {
		return DictionaryNamespacePage{}, err
	}
	items := make([]metadto.DictionaryNamespaceResponse, 0, len(rows))
	for i := range rows {
		items = append(items, mapNamespace(&rows[i], localeOrDefault(in.Locale), 0))
	}
	return DictionaryNamespacePage{Items: items, Total: total, Page: normalizedPage(in.Page), PageSize: normalizedPageSize(in.PageSize)}, nil
}

func (s *Service) CreateDictionaryItem(ctx context.Context, in CreateDictionaryItemInput) (metadto.DictionaryItemResponse, error) {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return metadto.DictionaryItemResponse{}, err
	}
	if strings.TrimSpace(in.NamespaceUUID) == "" {
		return metadto.DictionaryItemResponse{}, ErrUUIDRequired
	}
	if _, err := s.dictionaryRepo().GetNamespace(ctx, tenantUUID, strings.TrimSpace(in.NamespaceUUID)); err != nil {
		return metadto.DictionaryItemResponse{}, err
	}
	code := strings.TrimSpace(in.Code)
	if err := ValidateMachineIdentifier(code); err != nil {
		return metadto.DictionaryItemResponse{}, err
	}
	if err := ValidateRequiredI18n(in.LabelI18n, "zh-CN"); err != nil {
		return metadto.DictionaryItemResponse{}, err
	}
	row := &model.DictionaryItem{
		TenantUUID:      tenantUUID,
		NamespaceUUID:   strings.TrimSpace(in.NamespaceUUID),
		Code:            code,
		LabelI18n:       mustJSON(in.LabelI18n),
		DescriptionI18n: mustJSON(in.DescriptionI18n),
		SortOrder:       in.SortOrder,
		Status:          model.StatusEnabled,
		Metadata:        mustJSONAny(in.Metadata),
	}
	if err := s.dictionaryRepo().CreateItem(ctx, row); err != nil {
		return metadto.DictionaryItemResponse{}, err
	}
	s.publishAudit(ctx, AuditEvent{TenantUUID: tenantUUID, Operation: "create", ObjectType: "dictionary_item", ObjectUUID: row.UUID.String()})
	return mapItem(row, "zh-CN"), nil
}

func (s *Service) UpdateDictionaryItem(ctx context.Context, in UpdateDictionaryItemInput) (metadto.DictionaryItemResponse, error) {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return metadto.DictionaryItemResponse{}, err
	}
	itemUUID := strings.TrimSpace(in.ItemUUID)
	if itemUUID == "" {
		return metadto.DictionaryItemResponse{}, ErrUUIDRequired
	}
	updates := map[string]any{}
	if in.LabelI18n != nil {
		if err := ValidateRequiredI18n(*in.LabelI18n, "zh-CN"); err != nil {
			return metadto.DictionaryItemResponse{}, err
		}
		updates["label_i18n"] = mustJSON(*in.LabelI18n)
	}
	if in.DescriptionI18n != nil {
		updates["description_i18n"] = mustJSON(*in.DescriptionI18n)
	}
	if in.SortOrder != nil {
		updates["sort_order"] = *in.SortOrder
	}
	if in.Status != nil {
		status := strings.TrimSpace(*in.Status)
		if err := ValidateStatus(status); err != nil {
			return metadto.DictionaryItemResponse{}, err
		}
		updates["status"] = status
	}
	if in.Metadata != nil {
		updates["metadata"] = mustJSONAny(*in.Metadata)
	}
	row, err := s.dictionaryRepo().UpdateItem(ctx, tenantUUID, itemUUID, updates)
	if err != nil {
		return metadto.DictionaryItemResponse{}, err
	}
	s.publishAudit(ctx, AuditEvent{TenantUUID: tenantUUID, Operation: "update", ObjectType: "dictionary_item", ObjectUUID: row.UUID.String()})
	return mapItem(row, "zh-CN"), nil
}

func (s *Service) ListDictionaryItems(ctx context.Context, in ListDictionaryItemsInput) (DictionaryItemPage, error) {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return DictionaryItemPage{}, err
	}
	if strings.TrimSpace(in.NamespaceUUID) == "" {
		return DictionaryItemPage{}, ErrUUIDRequired
	}
	if strings.TrimSpace(in.Status) != "" {
		if err := ValidateStatus(strings.TrimSpace(in.Status)); err != nil {
			return DictionaryItemPage{}, err
		}
	}
	rows, total, err := s.dictionaryRepo().ListItems(ctx, metarepo.DictionaryItemListOptions{
		TenantUUID:    tenantUUID,
		NamespaceUUID: strings.TrimSpace(in.NamespaceUUID),
		Status:        in.Status,
		Query:         in.Query,
		Page:          normalizedPage(in.Page),
		PageSize:      normalizedPageSize(in.PageSize),
	})
	if err != nil {
		return DictionaryItemPage{}, err
	}
	items := make([]metadto.DictionaryItemResponse, 0, len(rows))
	for i := range rows {
		items = append(items, mapItem(&rows[i], localeOrDefault(in.Locale)))
	}
	return DictionaryItemPage{Items: items, Total: total, Page: normalizedPage(in.Page), PageSize: normalizedPageSize(in.PageSize)}, nil
}

func (s *Service) DeleteDictionaryItem(ctx context.Context, in DeleteDictionaryItemInput) ([]metadto.ReferenceSummary, error) {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return nil, err
	}
	itemUUID := strings.TrimSpace(in.ItemUUID)
	if itemUUID == "" {
		return nil, ErrUUIDRequired
	}
	total, refs, err := s.dictionaryRepo().CountItemReferences(ctx, tenantUUID, itemUUID)
	if err != nil {
		return nil, err
	}
	if total > 0 {
		return mapReferenceSummaries(refs, total), ErrReferenceConflict
	}
	if err := s.dictionaryRepo().DeleteItem(ctx, tenantUUID, itemUUID); err != nil {
		return nil, err
	}
	s.publishAudit(ctx, AuditEvent{TenantUUID: tenantUUID, Operation: "delete", ObjectType: "dictionary_item", ObjectUUID: itemUUID})
	return nil, nil
}

func canonicalTenant(raw string) (string, error) {
	return reqctx.CanonicalTenantUUID(raw)
}

func mustJSON(values map[string]string) datatypes.JSON {
	if values == nil {
		values = map[string]string{}
	}
	raw, _ := json.Marshal(values)
	return datatypes.JSON(raw)
}

func mustJSONAny(values map[string]any) datatypes.JSON {
	if values == nil {
		values = map[string]any{}
	}
	raw, _ := json.Marshal(values)
	return datatypes.JSON(raw)
}

func mapStringJSON(raw datatypes.JSON) map[string]string {
	var values map[string]string
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &values)
	}
	if values == nil {
		values = map[string]string{}
	}
	return values
}

func mapNamespace(row *model.DictionaryNamespace, locale string, itemCount int64) metadto.DictionaryNamespaceResponse {
	name := mapStringJSON(row.NameI18n)
	desc := mapStringJSON(row.DescriptionI18n)
	displayName, missing := localized(name, locale)
	displayDesc, _ := localized(desc, locale)
	return metadto.DictionaryNamespaceResponse{
		UUID:            row.UUID.String(),
		Namespace:       row.Namespace,
		Module:          row.Module,
		NameI18n:        name,
		DescriptionI18n: desc,
		Status:          row.Status,
		ItemCount:       itemCount,
		Display: metadto.Display{
			DisplayName:          displayName,
			DisplayDescription:   displayDesc,
			DisplayLocale:        locale,
			DisplayLocaleMissing: missing,
		},
	}
}

func mapItem(row *model.DictionaryItem, locale string) metadto.DictionaryItemResponse {
	label := mapStringJSON(row.LabelI18n)
	desc := mapStringJSON(row.DescriptionI18n)
	displayName, missing := localized(label, locale)
	displayDesc, _ := localized(desc, locale)
	return metadto.DictionaryItemResponse{
		UUID:            row.UUID.String(),
		NamespaceUUID:   row.NamespaceUUID,
		Code:            row.Code,
		LabelI18n:       label,
		DescriptionI18n: desc,
		Status:          row.Status,
		SortOrder:       row.SortOrder,
		ReferenceCount:  row.ReferenceCount,
		Display: metadto.Display{
			DisplayName:          displayName,
			DisplayDescription:   displayDesc,
			DisplayLocale:        locale,
			DisplayLocaleMissing: missing,
		},
	}
}

func localized(values map[string]string, locale string) (string, bool) {
	locale = localeOrDefault(locale)
	if v := strings.TrimSpace(values[locale]); v != "" {
		return v, false
	}
	return strings.TrimSpace(values["zh-CN"]), locale != "zh-CN"
}

func localeOrDefault(locale string) string {
	if strings.TrimSpace(locale) == "" {
		return "zh-CN"
	}
	return strings.TrimSpace(locale)
}

func normalizedPage(page int) int {
	if page <= 0 {
		return 1
	}
	return page
}

func normalizedPageSize(pageSize int) int {
	if pageSize <= 0 {
		return 20
	}
	if pageSize > 100 {
		return 100
	}
	return pageSize
}

func mapReferenceSummaries(refs []model.Reference, total int64) []metadto.ReferenceSummary {
	if len(refs) == 0 && total > 0 {
		return []metadto.ReferenceSummary{{Count: total}}
	}
	out := make([]metadto.ReferenceSummary, 0, len(refs))
	for _, ref := range refs {
		out = append(out, metadto.ReferenceSummary{
			ResourceType: ref.ResourceType,
			ResourceUUID: ref.ResourceUUID,
			FieldName:    ref.FieldName,
			Count:        1,
		})
	}
	return out
}

func isNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
