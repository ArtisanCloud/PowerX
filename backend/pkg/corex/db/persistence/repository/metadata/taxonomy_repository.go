package metadata

import (
	"context"
	"errors"
	"strings"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	"gorm.io/gorm"
)

var ErrOptimisticConflict = errors.New("metadata.optimistic_conflict")

type TaxonomyRepository struct {
	*Repository
}

func NewTaxonomyRepository(db *gorm.DB) *TaxonomyRepository {
	return &TaxonomyRepository{Repository: NewRepository(db)}
}

type TaxonomyListOptions struct {
	TenantUUID string
	Module     string
	Status     string
	Query      string
	Page       int
	PageSize   int
}

type TaxonomyNodeListOptions struct {
	TenantUUID   string
	TaxonomyUUID string
	Status       string
	Query        string
	Page         int
	PageSize     int
}

func (r *TaxonomyRepository) CreateTaxonomy(ctx context.Context, row *model.Taxonomy) error {
	return r.DB().WithContext(ctx).Create(row).Error
}

func (r *TaxonomyRepository) UpsertTaxonomyByNamespace(ctx context.Context, row *model.Taxonomy) (*model.Taxonomy, error) {
	var out model.Taxonomy
	err := r.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("tenant_uuid = ? AND namespace = ?", row.TenantUUID, row.Namespace).First(&out).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(row).Error; err != nil {
				return err
			}
			out = *row
			return nil
		}
		if err != nil {
			return err
		}
		updates := map[string]any{
			"module":           row.Module,
			"name_i18n":        row.NameI18n,
			"description_i18n": row.DescriptionI18n,
			"max_depth":        row.MaxDepth,
			"status":           row.Status,
		}
		if err := tx.Model(&out).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Where("tenant_uuid = ? AND namespace = ?", row.TenantUUID, row.Namespace).First(&out).Error
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *TaxonomyRepository) UpdateTaxonomy(ctx context.Context, tenantUUID, taxonomyUUID string, updates map[string]any) (*model.Taxonomy, error) {
	var row model.Taxonomy
	err := r.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_uuid = ? AND uuid = ?", tenantUUID, taxonomyUUID).First(&row).Error; err != nil {
			return err
		}
		if len(updates) > 0 {
			if err := tx.Model(&row).Updates(updates).Error; err != nil {
				return err
			}
			return tx.Where("tenant_uuid = ? AND uuid = ?", tenantUUID, taxonomyUUID).First(&row).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *TaxonomyRepository) GetTaxonomy(ctx context.Context, tenantUUID, taxonomyUUID string) (*model.Taxonomy, error) {
	var row model.Taxonomy
	if err := r.DB().WithContext(ctx).Where("tenant_uuid = ? AND uuid = ?", tenantUUID, taxonomyUUID).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *TaxonomyRepository) GetTaxonomyByNamespace(ctx context.Context, tenantUUID, namespace string) (*model.Taxonomy, error) {
	var row model.Taxonomy
	if err := r.DB().WithContext(ctx).Where("tenant_uuid = ? AND namespace = ?", tenantUUID, namespace).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *TaxonomyRepository) ListTaxonomies(ctx context.Context, opt TaxonomyListOptions) ([]model.Taxonomy, int64, error) {
	q := r.DB().WithContext(ctx).Model(&model.Taxonomy{}).Where("tenant_uuid = ?", opt.TenantUUID)
	if module := strings.TrimSpace(opt.Module); module != "" {
		q = q.Where("module = ?", module)
	}
	if status := strings.TrimSpace(opt.Status); status != "" {
		q = q.Where("status = ?", status)
	}
	if query := strings.TrimSpace(opt.Query); query != "" {
		like := "%" + strings.ToLower(query) + "%"
		q = q.Where("LOWER(namespace) LIKE ? OR LOWER(module) LIKE ? OR LOWER(CAST(name_i18n AS TEXT)) LIKE ?", like, like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.Taxonomy
	err := q.Order("module ASC, namespace ASC").Offset(offset(opt.Page, opt.PageSize)).Limit(limit(opt.PageSize)).Find(&rows).Error
	return rows, total, err
}

func (r *TaxonomyRepository) CreateNode(ctx context.Context, row *model.TaxonomyNode) error {
	return r.DB().WithContext(ctx).Create(row).Error
}

func (r *TaxonomyRepository) UpsertNodeByCode(ctx context.Context, row *model.TaxonomyNode) (*model.TaxonomyNode, error) {
	var out model.TaxonomyNode
	err := r.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("tenant_uuid = ? AND taxonomy_uuid = ? AND code = ?", row.TenantUUID, row.TaxonomyUUID, row.Code).First(&out).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(row).Error; err != nil {
				return err
			}
			out = *row
			return nil
		}
		if err != nil {
			return err
		}
		updates := map[string]any{
			"parent_uuid":      row.ParentUUID,
			"label_i18n":       row.LabelI18n,
			"description_i18n": row.DescriptionI18n,
			"path":             row.Path,
			"depth":            row.Depth,
			"sort_order":       row.SortOrder,
			"status":           row.Status,
			"version":          out.Version + 1,
		}
		if err := tx.Model(&out).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Where("tenant_uuid = ? AND taxonomy_uuid = ? AND code = ?", row.TenantUUID, row.TaxonomyUUID, row.Code).First(&out).Error
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *TaxonomyRepository) GetNode(ctx context.Context, tenantUUID, nodeUUID string) (*model.TaxonomyNode, error) {
	var row model.TaxonomyNode
	if err := r.DB().WithContext(ctx).Where("tenant_uuid = ? AND uuid = ?", tenantUUID, nodeUUID).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *TaxonomyRepository) ListNodes(ctx context.Context, opt TaxonomyNodeListOptions) ([]model.TaxonomyNode, error) {
	rows, _, err := r.ListNodesPage(ctx, opt)
	return rows, err
}

func (r *TaxonomyRepository) ListNodesPage(ctx context.Context, opt TaxonomyNodeListOptions) ([]model.TaxonomyNode, int64, error) {
	q := r.DB().WithContext(ctx).Model(&model.TaxonomyNode{}).
		Where("tenant_uuid = ? AND taxonomy_uuid = ?", opt.TenantUUID, opt.TaxonomyUUID)
	if status := strings.TrimSpace(opt.Status); status != "" {
		q = q.Where("status = ?", status)
	}
	if query := strings.TrimSpace(opt.Query); query != "" {
		like := "%" + strings.ToLower(query) + "%"
		q = q.Where("LOWER(code) LIKE ? OR LOWER(CAST(label_i18n AS TEXT)) LIKE ?", like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	pageSize := limit(opt.PageSize)
	var rows []model.TaxonomyNode
	err := q.Order("path ASC, sort_order ASC, code ASC").Offset(offset(opt.Page, opt.PageSize)).Limit(pageSize).Find(&rows).Error
	return rows, total, err
}

func (r *TaxonomyRepository) UpdateNode(ctx context.Context, tenantUUID, nodeUUID string, version int64, updates map[string]any) (*model.TaxonomyNode, error) {
	var row model.TaxonomyNode
	err := r.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_uuid = ? AND uuid = ?", tenantUUID, nodeUUID).First(&row).Error; err != nil {
			return err
		}
		if version > 0 && row.Version != version {
			return ErrOptimisticConflict
		}
		if len(updates) > 0 {
			updates["version"] = row.Version + 1
			if err := tx.Model(&row).Updates(updates).Error; err != nil {
				return err
			}
			return tx.Where("tenant_uuid = ? AND uuid = ?", tenantUUID, nodeUUID).First(&row).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *TaxonomyRepository) MoveNode(ctx context.Context, tenantUUID, nodeUUID string, version int64, parentUUID *string, sortOrder int, newPath string, newDepth int, descendants []model.TaxonomyNode) (*model.TaxonomyNode, error) {
	var row model.TaxonomyNode
	err := r.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_uuid = ? AND uuid = ?", tenantUUID, nodeUUID).First(&row).Error; err != nil {
			return err
		}
		if version > 0 && row.Version != version {
			return ErrOptimisticConflict
		}
		updates := map[string]any{
			"parent_uuid": parentUUID,
			"sort_order":  sortOrder,
			"path":        newPath,
			"depth":       newDepth,
			"version":     row.Version + 1,
		}
		if err := tx.Model(&row).Updates(updates).Error; err != nil {
			return err
		}
		for i := range descendants {
			desc := descendants[i]
			if err := tx.Model(&model.TaxonomyNode{}).
				Where("tenant_uuid = ? AND uuid = ?", tenantUUID, desc.UUID.String()).
				Updates(map[string]any{"path": desc.Path, "depth": desc.Depth, "version": desc.Version + 1}).Error; err != nil {
				return err
			}
		}
		return tx.Where("tenant_uuid = ? AND uuid = ?", tenantUUID, nodeUUID).First(&row).Error
	})
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *TaxonomyRepository) DeleteNode(ctx context.Context, tenantUUID, nodeUUID string) error {
	return r.DB().WithContext(ctx).Where("tenant_uuid = ? AND uuid = ?", tenantUUID, nodeUUID).Delete(&model.TaxonomyNode{}).Error
}

func (r *TaxonomyRepository) CountNodeReferences(ctx context.Context, tenantUUID, nodeUUID string) (int64, []model.Reference, error) {
	var total int64
	q := r.DB().WithContext(ctx).Model(&model.Reference{}).
		Where("tenant_uuid = ? AND metadata_type = ? AND metadata_uuid = ?", tenantUUID, model.MetadataTypeTaxonomyNode, nodeUUID)
	if err := q.Count(&total).Error; err != nil {
		return 0, nil, err
	}
	var refs []model.Reference
	if total > 0 {
		if err := q.Limit(20).Find(&refs).Error; err != nil {
			return 0, nil, err
		}
	}
	return total, refs, nil
}
