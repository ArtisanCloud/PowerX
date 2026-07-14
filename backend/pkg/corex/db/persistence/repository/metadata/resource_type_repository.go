package metadata

import (
	"context"
	"errors"
	"strings"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	"gorm.io/gorm"
)

type ResourceTypeRepository struct {
	*Repository
}

func NewResourceTypeRepository(db *gorm.DB) *ResourceTypeRepository {
	return &ResourceTypeRepository{Repository: NewRepository(db)}
}

type ResourceTypeListOptions struct {
	TenantUUID string
	Module     string
	Status     string
	Query      string
	Page       int
	PageSize   int
}

func (r *ResourceTypeRepository) Create(ctx context.Context, row *model.ResourceType) error {
	return r.DB().WithContext(ctx).Create(row).Error
}

func (r *ResourceTypeRepository) UpsertByResourceType(ctx context.Context, row *model.ResourceType) (*model.ResourceType, error) {
	var out model.ResourceType
	err := r.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("tenant_uuid = ? AND resource_type = ?", row.TenantUUID, row.ResourceType).First(&out).Error
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
			"validator_key":    row.ValidatorKey,
			"binding_enabled":  row.BindingEnabled,
			"status":           row.Status,
		}
		if err := tx.Model(&out).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Where("tenant_uuid = ? AND resource_type = ?", row.TenantUUID, row.ResourceType).First(&out).Error
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *ResourceTypeRepository) Get(ctx context.Context, tenantUUID, resourceTypeUUID string) (*model.ResourceType, error) {
	var row model.ResourceType
	if err := r.DB().WithContext(ctx).Where("tenant_uuid = ? AND uuid = ?", tenantUUID, resourceTypeUUID).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *ResourceTypeRepository) GetByResourceType(ctx context.Context, tenantUUID, resourceType string) (*model.ResourceType, error) {
	var row model.ResourceType
	if err := r.DB().WithContext(ctx).Where("tenant_uuid = ? AND resource_type = ?", tenantUUID, resourceType).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *ResourceTypeRepository) List(ctx context.Context, opt ResourceTypeListOptions) ([]model.ResourceType, int64, error) {
	q := r.DB().WithContext(ctx).Model(&model.ResourceType{}).Where("tenant_uuid = ?", opt.TenantUUID)
	if module := strings.TrimSpace(opt.Module); module != "" {
		q = q.Where("module = ?", module)
	}
	if status := strings.TrimSpace(opt.Status); status != "" {
		q = q.Where("status = ?", status)
	}
	if query := strings.TrimSpace(opt.Query); query != "" {
		like := "%" + strings.ToLower(query) + "%"
		q = q.Where("LOWER(resource_type) LIKE ? OR LOWER(module) LIKE ? OR LOWER(CAST(name_i18n AS TEXT)) LIKE ?", like, like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.ResourceType
	err := q.Order("module ASC, resource_type ASC").Offset(offset(opt.Page, opt.PageSize)).Limit(limit(opt.PageSize)).Find(&rows).Error
	return rows, total, err
}

func (r *ResourceTypeRepository) Update(ctx context.Context, tenantUUID, resourceTypeUUID string, updates map[string]any) (*model.ResourceType, error) {
	var row model.ResourceType
	err := r.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_uuid = ? AND uuid = ?", tenantUUID, resourceTypeUUID).First(&row).Error; err != nil {
			return err
		}
		if len(updates) > 0 {
			if err := tx.Model(&row).Updates(updates).Error; err != nil {
				return err
			}
			return tx.Where("tenant_uuid = ? AND uuid = ?", tenantUUID, resourceTypeUUID).First(&row).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &row, nil
}
