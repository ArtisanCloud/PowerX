package metadata

import (
	"context"
	"errors"
	"fmt"
	"strings"

	metadto "github.com/ArtisanCloud/PowerX/internal/dto/metadata"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	metarepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/metadata"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrInvalidDepth       = errors.New("metadata.invalid_depth")
	ErrCircularMove       = errors.New("metadata.circular_move")
	ErrOptimisticConflict = errors.New("metadata.optimistic_conflict")
	ErrHasChildNodes      = errors.New("metadata.has_child_nodes")
)

type CreateTaxonomyInput struct {
	TenantUUID      string
	Namespace       string
	Module          string
	NameI18n        map[string]string
	DescriptionI18n map[string]string
	MaxDepth        int
}

type UpdateTaxonomyInput struct {
	TenantUUID      string
	TaxonomyUUID    string
	NameI18n        *map[string]string
	DescriptionI18n *map[string]string
	MaxDepth        *int
	Status          *string
}

type ListTaxonomiesInput struct {
	TenantUUID string
	Module     string
	Status     string
	Query      string
	Locale     string
	Page       int
	PageSize   int
}

type CreateTaxonomyNodeInput struct {
	TenantUUID      string
	TaxonomyUUID    string
	ParentUUID      *string
	Code            string
	LabelI18n       map[string]string
	DescriptionI18n map[string]string
	SortOrder       int
}

type UpdateTaxonomyNodeInput struct {
	TenantUUID      string
	NodeUUID        string
	LabelI18n       *map[string]string
	DescriptionI18n *map[string]string
	SortOrder       *int
	Status          *string
	Version         int64
}

type MoveTaxonomyNodeInput struct {
	TenantUUID       string
	NodeUUID         string
	TargetParentUUID *string
	SortOrder        *int
	Version          int64
}

type ListTaxonomyNodesInput struct {
	TenantUUID   string
	TaxonomyUUID string
	Status       string
	Query        string
	Locale       string
	Page         int
	PageSize     int
}

type DeleteTaxonomyNodeInput struct {
	TenantUUID string
	NodeUUID   string
}

type TaxonomyPage struct {
	Items    []metadto.TaxonomyResponse
	Total    int64
	Page     int
	PageSize int
}

type TaxonomyNodePage struct {
	Items    []metadto.TaxonomyNodeResponse
	Total    int64
	Page     int
	PageSize int
}

func (s *Service) taxonomyRepo() *metarepo.TaxonomyRepository {
	return metarepo.NewTaxonomyRepository(s.deps.DB)
}

func (s *Service) CreateTaxonomy(ctx context.Context, in CreateTaxonomyInput) (metadto.TaxonomyResponse, error) {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return metadto.TaxonomyResponse{}, err
	}
	namespace := strings.TrimSpace(in.Namespace)
	if err := ValidateMachineIdentifier(namespace); err != nil {
		return metadto.TaxonomyResponse{}, err
	}
	module := strings.TrimSpace(in.Module)
	if err := ValidateNamespaceInModule(namespace, module); err != nil {
		return metadto.TaxonomyResponse{}, err
	}
	if err := ValidateRequiredI18n(in.NameI18n, "zh-CN"); err != nil {
		return metadto.TaxonomyResponse{}, err
	}
	if in.MaxDepth < 1 {
		return metadto.TaxonomyResponse{}, ErrInvalidDepth
	}
	if _, err := s.taxonomyRepo().GetTaxonomyByNamespace(ctx, tenantUUID, namespace); err == nil {
		return metadto.TaxonomyResponse{}, ErrAlreadyExists
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return metadto.TaxonomyResponse{}, err
	}
	row := &model.Taxonomy{
		TenantUUID:      tenantUUID,
		Namespace:       namespace,
		Module:          module,
		NameI18n:        mustJSON(in.NameI18n),
		DescriptionI18n: mustJSON(in.DescriptionI18n),
		MaxDepth:        in.MaxDepth,
		Status:          model.StatusEnabled,
	}
	if err := s.taxonomyRepo().CreateTaxonomy(ctx, row); err != nil {
		return metadto.TaxonomyResponse{}, err
	}
	s.publishAudit(ctx, AuditEvent{TenantUUID: tenantUUID, Operation: "create", ObjectType: "taxonomy", ObjectUUID: row.UUID.String()})
	return mapTaxonomy(row, "zh-CN"), nil
}

func (s *Service) UpdateTaxonomy(ctx context.Context, in UpdateTaxonomyInput) (metadto.TaxonomyResponse, error) {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return metadto.TaxonomyResponse{}, err
	}
	taxonomyUUID := strings.TrimSpace(in.TaxonomyUUID)
	if taxonomyUUID == "" {
		return metadto.TaxonomyResponse{}, ErrUUIDRequired
	}
	updates := map[string]any{}
	if in.NameI18n != nil {
		if err := ValidateRequiredI18n(*in.NameI18n, "zh-CN"); err != nil {
			return metadto.TaxonomyResponse{}, err
		}
		updates["name_i18n"] = mustJSON(*in.NameI18n)
	}
	if in.DescriptionI18n != nil {
		updates["description_i18n"] = mustJSON(*in.DescriptionI18n)
	}
	if in.MaxDepth != nil {
		if *in.MaxDepth < 1 {
			return metadto.TaxonomyResponse{}, ErrInvalidDepth
		}
		nodes, err := s.taxonomyRepo().ListNodes(ctx, metarepo.TaxonomyNodeListOptions{TenantUUID: tenantUUID, TaxonomyUUID: taxonomyUUID})
		if err != nil {
			return metadto.TaxonomyResponse{}, err
		}
		for i := range nodes {
			if nodes[i].Depth > *in.MaxDepth {
				return metadto.TaxonomyResponse{}, ErrInvalidDepth
			}
		}
		updates["max_depth"] = *in.MaxDepth
	}
	if in.Status != nil {
		status := strings.TrimSpace(*in.Status)
		if err := ValidateStatus(status); err != nil {
			return metadto.TaxonomyResponse{}, err
		}
		updates["status"] = status
	}
	row, err := s.taxonomyRepo().UpdateTaxonomy(ctx, tenantUUID, taxonomyUUID, updates)
	if err != nil {
		return metadto.TaxonomyResponse{}, err
	}
	s.publishAudit(ctx, AuditEvent{TenantUUID: tenantUUID, Operation: "update", ObjectType: "taxonomy", ObjectUUID: row.UUID.String()})
	return mapTaxonomy(row, "zh-CN"), nil
}

func (s *Service) ListTaxonomies(ctx context.Context, in ListTaxonomiesInput) (TaxonomyPage, error) {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return TaxonomyPage{}, err
	}
	if strings.TrimSpace(in.Status) != "" {
		if err := ValidateStatus(strings.TrimSpace(in.Status)); err != nil {
			return TaxonomyPage{}, err
		}
	}
	rows, total, err := s.taxonomyRepo().ListTaxonomies(ctx, metarepo.TaxonomyListOptions{
		TenantUUID: tenantUUID,
		Module:     in.Module,
		Status:     in.Status,
		Query:      in.Query,
		Page:       normalizedPage(in.Page),
		PageSize:   normalizedPageSize(in.PageSize),
	})
	if err != nil {
		return TaxonomyPage{}, err
	}
	items := make([]metadto.TaxonomyResponse, 0, len(rows))
	for i := range rows {
		items = append(items, mapTaxonomy(&rows[i], localeOrDefault(in.Locale)))
	}
	return TaxonomyPage{Items: items, Total: total, Page: normalizedPage(in.Page), PageSize: normalizedPageSize(in.PageSize)}, nil
}

func (s *Service) CreateTaxonomyNode(ctx context.Context, in CreateTaxonomyNodeInput) (metadto.TaxonomyNodeResponse, error) {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return metadto.TaxonomyNodeResponse{}, err
	}
	taxonomy, err := s.taxonomyRepo().GetTaxonomy(ctx, tenantUUID, strings.TrimSpace(in.TaxonomyUUID))
	if err != nil {
		return metadto.TaxonomyNodeResponse{}, err
	}
	code := strings.TrimSpace(in.Code)
	if err := ValidateMachineIdentifier(code); err != nil {
		return metadto.TaxonomyNodeResponse{}, err
	}
	if err := ValidateRequiredI18n(in.LabelI18n, "zh-CN"); err != nil {
		return metadto.TaxonomyNodeResponse{}, err
	}
	parent, depth, err := s.resolveTaxonomyParent(ctx, tenantUUID, taxonomy, in.ParentUUID)
	if err != nil {
		return metadto.TaxonomyNodeResponse{}, err
	}
	if depth > taxonomy.MaxDepth {
		return metadto.TaxonomyNodeResponse{}, ErrInvalidDepth
	}
	nodeUUID := uuid.New()
	path := taxonomyNodePath(taxonomy.UUID.String(), nodeUUID.String(), parent)
	row := &model.TaxonomyNode{
		PowerUUIDModel:  coremodel.PowerUUIDModel{UUID: nodeUUID},
		TenantUUID:      tenantUUID,
		TaxonomyUUID:    taxonomy.UUID.String(),
		ParentUUID:      cleanOptionalUUID(in.ParentUUID),
		Code:            code,
		LabelI18n:       mustJSON(in.LabelI18n),
		DescriptionI18n: mustJSON(in.DescriptionI18n),
		Path:            path,
		Depth:           depth,
		SortOrder:       in.SortOrder,
		Status:          model.StatusEnabled,
		Version:         1,
	}
	if err := s.taxonomyRepo().CreateNode(ctx, row); err != nil {
		return metadto.TaxonomyNodeResponse{}, err
	}
	s.publishAudit(ctx, AuditEvent{TenantUUID: tenantUUID, Operation: "create", ObjectType: "taxonomy_node", ObjectUUID: row.UUID.String()})
	return mapTaxonomyNode(row, "zh-CN"), nil
}

func (s *Service) ListTaxonomyNodes(ctx context.Context, in ListTaxonomyNodesInput) ([]metadto.TaxonomyNodeResponse, error) {
	page, err := s.ListTaxonomyNodesPage(ctx, in)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (s *Service) ListTaxonomyNodesPage(ctx context.Context, in ListTaxonomyNodesInput) (TaxonomyNodePage, error) {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return TaxonomyNodePage{}, err
	}
	if strings.TrimSpace(in.TaxonomyUUID) == "" {
		return TaxonomyNodePage{}, ErrUUIDRequired
	}
	if strings.TrimSpace(in.Status) != "" {
		if err := ValidateStatus(strings.TrimSpace(in.Status)); err != nil {
			return TaxonomyNodePage{}, err
		}
	}
	rows, total, err := s.taxonomyRepo().ListNodesPage(ctx, metarepo.TaxonomyNodeListOptions{
		TenantUUID:   tenantUUID,
		TaxonomyUUID: strings.TrimSpace(in.TaxonomyUUID),
		Status:       in.Status,
		Query:        in.Query,
		Page:         normalizedPage(in.Page),
		PageSize:     normalizedPageSize(in.PageSize),
	})
	if err != nil {
		return TaxonomyNodePage{}, err
	}
	items := make([]metadto.TaxonomyNodeResponse, 0, len(rows))
	for i := range rows {
		items = append(items, mapTaxonomyNode(&rows[i], localeOrDefault(in.Locale)))
	}
	return TaxonomyNodePage{Items: items, Total: total, Page: normalizedPage(in.Page), PageSize: normalizedPageSize(in.PageSize)}, nil
}

func (s *Service) UpdateTaxonomyNode(ctx context.Context, in UpdateTaxonomyNodeInput) (metadto.TaxonomyNodeResponse, error) {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return metadto.TaxonomyNodeResponse{}, err
	}
	if strings.TrimSpace(in.NodeUUID) == "" {
		return metadto.TaxonomyNodeResponse{}, ErrUUIDRequired
	}
	updates := map[string]any{}
	if in.LabelI18n != nil {
		if err := ValidateRequiredI18n(*in.LabelI18n, "zh-CN"); err != nil {
			return metadto.TaxonomyNodeResponse{}, err
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
			return metadto.TaxonomyNodeResponse{}, err
		}
		updates["status"] = status
	}
	row, err := s.taxonomyRepo().UpdateNode(ctx, tenantUUID, strings.TrimSpace(in.NodeUUID), in.Version, updates)
	if errors.Is(err, metarepo.ErrOptimisticConflict) {
		return metadto.TaxonomyNodeResponse{}, ErrOptimisticConflict
	}
	if err != nil {
		return metadto.TaxonomyNodeResponse{}, err
	}
	s.publishAudit(ctx, AuditEvent{TenantUUID: tenantUUID, Operation: "update", ObjectType: "taxonomy_node", ObjectUUID: row.UUID.String()})
	return mapTaxonomyNode(row, "zh-CN"), nil
}

func (s *Service) MoveTaxonomyNode(ctx context.Context, in MoveTaxonomyNodeInput) (metadto.TaxonomyNodeResponse, error) {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return metadto.TaxonomyNodeResponse{}, err
	}
	node, err := s.taxonomyRepo().GetNode(ctx, tenantUUID, strings.TrimSpace(in.NodeUUID))
	if err != nil {
		return metadto.TaxonomyNodeResponse{}, err
	}
	taxonomy, err := s.taxonomyRepo().GetTaxonomy(ctx, tenantUUID, node.TaxonomyUUID)
	if err != nil {
		return metadto.TaxonomyNodeResponse{}, err
	}
	targetParent, newDepth, err := s.resolveTaxonomyParent(ctx, tenantUUID, taxonomy, in.TargetParentUUID)
	if err != nil {
		return metadto.TaxonomyNodeResponse{}, err
	}
	if targetParent != nil && targetParent.UUID.String() == node.UUID.String() {
		return metadto.TaxonomyNodeResponse{}, ErrCircularMove
	}
	if targetParent != nil && strings.HasPrefix(targetParent.Path, node.Path+"/") {
		return metadto.TaxonomyNodeResponse{}, ErrCircularMove
	}
	descendants, err := s.taxonomyRepo().ListNodes(ctx, metarepo.TaxonomyNodeListOptions{TenantUUID: tenantUUID, TaxonomyUUID: node.TaxonomyUUID})
	if err != nil {
		return metadto.TaxonomyNodeResponse{}, err
	}
	newPath := taxonomyNodePath(taxonomy.UUID.String(), node.UUID.String(), targetParent)
	updatedDescendants := make([]model.TaxonomyNode, 0)
	for i := range descendants {
		desc := descendants[i]
		if !strings.HasPrefix(desc.Path, node.Path+"/") {
			continue
		}
		suffix := strings.TrimPrefix(desc.Path, node.Path)
		desc.Path = newPath + suffix
		desc.Depth = newDepth + strings.Count(strings.Trim(suffix, "/"), "/") + 1
		if strings.Trim(suffix, "/") == "" {
			desc.Depth = newDepth
		}
		if desc.Depth > taxonomy.MaxDepth {
			return metadto.TaxonomyNodeResponse{}, ErrInvalidDepth
		}
		updatedDescendants = append(updatedDescendants, desc)
	}
	if newDepth > taxonomy.MaxDepth {
		return metadto.TaxonomyNodeResponse{}, ErrInvalidDepth
	}
	sortOrder := node.SortOrder
	if in.SortOrder != nil {
		sortOrder = *in.SortOrder
	}
	row, err := s.taxonomyRepo().MoveNode(ctx, tenantUUID, node.UUID.String(), in.Version, cleanOptionalUUID(in.TargetParentUUID), sortOrder, newPath, newDepth, updatedDescendants)
	if errors.Is(err, metarepo.ErrOptimisticConflict) {
		return metadto.TaxonomyNodeResponse{}, ErrOptimisticConflict
	}
	if err != nil {
		return metadto.TaxonomyNodeResponse{}, err
	}
	s.publishAudit(ctx, AuditEvent{TenantUUID: tenantUUID, Operation: "move", ObjectType: "taxonomy_node", ObjectUUID: row.UUID.String()})
	return mapTaxonomyNode(row, "zh-CN"), nil
}

func (s *Service) DeleteTaxonomyNode(ctx context.Context, in DeleteTaxonomyNodeInput) ([]metadto.ReferenceSummary, error) {
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return nil, err
	}
	nodeUUID := strings.TrimSpace(in.NodeUUID)
	if nodeUUID == "" {
		return nil, ErrUUIDRequired
	}
	node, err := s.taxonomyRepo().GetNode(ctx, tenantUUID, nodeUUID)
	if err != nil {
		return nil, err
	}
	nodes, err := s.taxonomyRepo().ListNodes(ctx, metarepo.TaxonomyNodeListOptions{TenantUUID: tenantUUID, TaxonomyUUID: node.TaxonomyUUID})
	if err != nil {
		return nil, err
	}
	for i := range nodes {
		if strings.TrimSpace(taxonomyNodeParentUUID(nodes[i])) == nodeUUID {
			return nil, ErrHasChildNodes
		}
	}
	total, refs, err := s.taxonomyRepo().CountNodeReferences(ctx, tenantUUID, nodeUUID)
	if err != nil {
		return nil, err
	}
	if total > 0 {
		return mapReferenceSummaries(refs, total), ErrReferenceConflict
	}
	if err := s.taxonomyRepo().DeleteNode(ctx, tenantUUID, nodeUUID); err != nil {
		return nil, err
	}
	s.publishAudit(ctx, AuditEvent{TenantUUID: tenantUUID, Operation: "delete", ObjectType: "taxonomy_node", ObjectUUID: nodeUUID})
	return nil, nil
}

func (s *Service) resolveTaxonomyParent(ctx context.Context, tenantUUID string, taxonomy *model.Taxonomy, parentUUID *string) (*model.TaxonomyNode, int, error) {
	parent := cleanOptionalUUID(parentUUID)
	if parent == nil {
		return nil, 1, nil
	}
	row, err := s.taxonomyRepo().GetNode(ctx, tenantUUID, *parent)
	if err != nil {
		return nil, 0, err
	}
	if row.TaxonomyUUID != taxonomy.UUID.String() {
		return nil, 0, ErrUUIDRequired
	}
	return row, row.Depth + 1, nil
}

func cleanOptionalUUID(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func taxonomyNodePath(taxonomyUUID string, nodeUUID string, parent *model.TaxonomyNode) string {
	if parent == nil {
		return fmt.Sprintf("/%s/%s", taxonomyUUID, nodeUUID)
	}
	return parent.Path + "/" + nodeUUID
}

func mapTaxonomy(row *model.Taxonomy, locale string) metadto.TaxonomyResponse {
	name := mapStringJSON(row.NameI18n)
	desc := mapStringJSON(row.DescriptionI18n)
	displayName, missing := localized(name, locale)
	displayDesc, _ := localized(desc, locale)
	return metadto.TaxonomyResponse{
		UUID:            row.UUID.String(),
		Namespace:       row.Namespace,
		Module:          row.Module,
		NameI18n:        name,
		DescriptionI18n: desc,
		MaxDepth:        row.MaxDepth,
		Status:          row.Status,
		Display: metadto.Display{
			DisplayName:          displayName,
			DisplayDescription:   displayDesc,
			DisplayLocale:        locale,
			DisplayLocaleMissing: missing,
		},
	}
}

func mapTaxonomyNode(row *model.TaxonomyNode, locale string) metadto.TaxonomyNodeResponse {
	label := mapStringJSON(row.LabelI18n)
	desc := mapStringJSON(row.DescriptionI18n)
	displayName, missing := localized(label, locale)
	displayDesc, _ := localized(desc, locale)
	return metadto.TaxonomyNodeResponse{
		UUID:            row.UUID.String(),
		TaxonomyUUID:    row.TaxonomyUUID,
		ParentUUID:      row.ParentUUID,
		Code:            row.Code,
		LabelI18n:       label,
		DescriptionI18n: desc,
		Path:            row.Path,
		Depth:           row.Depth,
		SortOrder:       row.SortOrder,
		Status:          row.Status,
		ReferenceCount:  row.ReferenceCount,
		Version:         row.Version,
		Display: metadto.Display{
			DisplayName:          displayName,
			DisplayDescription:   displayDesc,
			DisplayLocale:        locale,
			DisplayLocaleMissing: missing,
		},
	}
}

func taxonomyNodeParentUUID(node model.TaxonomyNode) string {
	if node.ParentUUID == nil {
		return ""
	}
	return *node.ParentUUID
}
