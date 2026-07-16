package metadata

import (
	"context"
	"errors"
	"strings"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type DictionaryRepository struct {
	*Repository
}

func NewDictionaryRepository(db *gorm.DB) *DictionaryRepository {
	return &DictionaryRepository{Repository: NewRepository(db)}
}

func (r *DictionaryRepository) BaseQuery(tenantUUID string) *gorm.DB {
	return r.DB().Model(&model.DictionaryNamespace{}).Where("tenant_uuid = ?", tenantUUID)
}

type DictionaryNamespaceListOptions struct {
	TenantUUID string
	Module     string
	Status     string
	Query      string
	Page       int
	PageSize   int
}

type DictionaryItemListOptions struct {
	TenantUUID    string
	NamespaceUUID string
	Status        string
	Query         string
	Page          int
	PageSize      int
}

func (r *DictionaryRepository) CreateNamespace(ctx context.Context, row *model.DictionaryNamespace) error {
	return r.DB().WithContext(ctx).Create(row).Error
}

func (r *DictionaryRepository) UpdateNamespace(ctx context.Context, tenantUUID, namespaceUUID string, updates map[string]any) (*model.DictionaryNamespace, error) {
	var row model.DictionaryNamespace
	err := r.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_uuid = ? AND uuid = ?", tenantUUID, namespaceUUID).First(&row).Error; err != nil {
			return err
		}
		if len(updates) > 0 {
			if err := tx.Model(&row).Updates(updates).Error; err != nil {
				return err
			}
			return tx.Where("tenant_uuid = ? AND uuid = ?", tenantUUID, namespaceUUID).First(&row).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *DictionaryRepository) GetNamespace(ctx context.Context, tenantUUID, namespaceUUID string) (*model.DictionaryNamespace, error) {
	var row model.DictionaryNamespace
	if err := r.DB().WithContext(ctx).Where("tenant_uuid = ? AND uuid = ?", tenantUUID, namespaceUUID).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *DictionaryRepository) GetNamespaceByNamespace(ctx context.Context, tenantUUID, namespace string) (*model.DictionaryNamespace, error) {
	var row model.DictionaryNamespace
	if err := r.DB().WithContext(ctx).Where("tenant_uuid = ? AND namespace = ?", tenantUUID, namespace).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *DictionaryRepository) UpsertNamespaceByNamespace(ctx context.Context, row *model.DictionaryNamespace) (*model.DictionaryNamespace, error) {
	var out model.DictionaryNamespace
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

func (r *DictionaryRepository) ListNamespaces(ctx context.Context, opt DictionaryNamespaceListOptions) ([]model.DictionaryNamespace, int64, error) {
	q := r.DB().WithContext(ctx).Model(&model.DictionaryNamespace{}).Where("tenant_uuid = ?", opt.TenantUUID)
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
	var rows []model.DictionaryNamespace
	err := q.Order("module ASC, namespace ASC").Offset(offset(opt.Page, opt.PageSize)).Limit(limit(opt.PageSize)).Find(&rows).Error
	return rows, total, err
}

func (r *DictionaryRepository) CreateItem(ctx context.Context, row *model.DictionaryItem) error {
	return r.DB().WithContext(ctx).Create(row).Error
}

func (r *DictionaryRepository) UpsertItemByCode(ctx context.Context, row *model.DictionaryItem) (*model.DictionaryItem, error) {
	var out model.DictionaryItem
	err := r.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("tenant_uuid = ? AND namespace_uuid = ? AND code = ?", row.TenantUUID, row.NamespaceUUID, row.Code).First(&out).Error
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
			"label_i18n":       row.LabelI18n,
			"description_i18n": row.DescriptionI18n,
			"sort_order":       row.SortOrder,
			"status":           row.Status,
			"metadata":         row.Metadata,
		}
		if err := tx.Model(&out).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Where("tenant_uuid = ? AND namespace_uuid = ? AND code = ?", row.TenantUUID, row.NamespaceUUID, row.Code).First(&out).Error
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *DictionaryRepository) UpdateItem(ctx context.Context, tenantUUID, itemUUID string, updates map[string]any) (*model.DictionaryItem, error) {
	var row model.DictionaryItem
	err := r.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_uuid = ? AND uuid = ?", tenantUUID, itemUUID).First(&row).Error; err != nil {
			return err
		}
		if len(updates) > 0 {
			if err := tx.Model(&row).Updates(updates).Error; err != nil {
				return err
			}
			return tx.Where("tenant_uuid = ? AND uuid = ?", tenantUUID, itemUUID).First(&row).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *DictionaryRepository) ListItems(ctx context.Context, opt DictionaryItemListOptions) ([]model.DictionaryItem, int64, error) {
	q := r.DB().WithContext(ctx).Model(&model.DictionaryItem{}).
		Where("tenant_uuid = ? AND namespace_uuid = ?", opt.TenantUUID, opt.NamespaceUUID)
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
	var rows []model.DictionaryItem
	err := q.Order("sort_order ASC, code ASC").Offset(offset(opt.Page, opt.PageSize)).Limit(limit(opt.PageSize)).Find(&rows).Error
	return rows, total, err
}

func (r *DictionaryRepository) DeleteItem(ctx context.Context, tenantUUID, itemUUID string) error {
	return r.DB().WithContext(ctx).Where("tenant_uuid = ? AND uuid = ?", tenantUUID, itemUUID).Delete(&model.DictionaryItem{}).Error
}

func (r *DictionaryRepository) CountItemReferences(ctx context.Context, tenantUUID, itemUUID string) (int64, []model.Reference, error) {
	var total int64
	q := r.DB().WithContext(ctx).Model(&model.Reference{}).
		Where("tenant_uuid = ? AND metadata_type = ? AND metadata_uuid = ?", tenantUUID, model.MetadataTypeDictionaryItem, itemUUID)
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

func (r *DictionaryRepository) EnsureNamespaceUUID(row *model.DictionaryNamespace) {
	if row != nil && row.UUID == uuid.Nil {
		row.UUID = uuid.New()
	}
}

func offset(page, pageSize int) int {
	if page <= 1 {
		return 0
	}
	return (page - 1) * limit(pageSize)
}

func limit(pageSize int) int {
	if pageSize <= 0 {
		return 20
	}
	if pageSize > 100 {
		return 100
	}
	return pageSize
}
