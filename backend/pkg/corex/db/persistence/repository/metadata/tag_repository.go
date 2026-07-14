package metadata

import (
	"context"
	"errors"
	"strings"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	"gorm.io/gorm"
)

type TagRepository struct {
	*Repository
}

func NewTagRepository(db *gorm.DB) *TagRepository {
	return &TagRepository{Repository: NewRepository(db)}
}

type TagListOptions struct {
	TenantUUID   string
	Namespace    string
	ResourceType string
	Status       string
	Query        string
	Page         int
	PageSize     int
}

func (r *TagRepository) CreateTag(ctx context.Context, row *model.Tag) error {
	return r.DB().WithContext(ctx).Create(row).Error
}

func (r *TagRepository) UpsertTagByCode(ctx context.Context, row *model.Tag) (*model.Tag, error) {
	var out model.Tag
	err := r.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("tenant_uuid = ? AND namespace = ? AND resource_type = ? AND code = ?", row.TenantUUID, row.Namespace, row.ResourceType, row.Code).First(&out).Error
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
			"color":            row.Color,
			"source":           row.Source,
			"status":           row.Status,
		}
		if err := tx.Model(&out).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Where("tenant_uuid = ? AND namespace = ? AND resource_type = ? AND code = ?", row.TenantUUID, row.Namespace, row.ResourceType, row.Code).First(&out).Error
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *TagRepository) GetTag(ctx context.Context, tenantUUID, tagUUID string) (*model.Tag, error) {
	var row model.Tag
	if err := r.DB().WithContext(ctx).Where("tenant_uuid = ? AND uuid = ?", tenantUUID, tagUUID).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *TagRepository) ListTags(ctx context.Context, opt TagListOptions) ([]model.Tag, int64, error) {
	q := r.DB().WithContext(ctx).Model(&model.Tag{}).Where("tenant_uuid = ?", opt.TenantUUID)
	if namespace := strings.TrimSpace(opt.Namespace); namespace != "" {
		q = q.Where("namespace = ?", namespace)
	}
	if resourceType := strings.TrimSpace(opt.ResourceType); resourceType != "" {
		q = q.Where("resource_type = ?", resourceType)
	}
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
	var rows []model.Tag
	err := q.Order("resource_type ASC, namespace ASC, code ASC").Offset(offset(opt.Page, opt.PageSize)).Limit(limit(opt.PageSize)).Find(&rows).Error
	return rows, total, err
}

func (r *TagRepository) UpdateTag(ctx context.Context, tenantUUID, tagUUID string, updates map[string]any) (*model.Tag, error) {
	var row model.Tag
	err := r.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_uuid = ? AND uuid = ?", tenantUUID, tagUUID).First(&row).Error; err != nil {
			return err
		}
		if len(updates) > 0 {
			if err := tx.Model(&row).Updates(updates).Error; err != nil {
				return err
			}
			return tx.Where("tenant_uuid = ? AND uuid = ?", tenantUUID, tagUUID).First(&row).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *TagRepository) DeleteTag(ctx context.Context, tenantUUID, tagUUID string) error {
	return r.DB().WithContext(ctx).Where("tenant_uuid = ? AND uuid = ?", tenantUUID, tagUUID).Delete(&model.Tag{}).Error
}

func (r *TagRepository) CountBindings(ctx context.Context, tenantUUID, tagUUID string) (int64, error) {
	var total int64
	err := r.DB().WithContext(ctx).Model(&model.TagBinding{}).Where("tenant_uuid = ? AND tag_uuid = ?", tenantUUID, tagUUID).Count(&total).Error
	return total, err
}

func (r *TagRepository) RecountUsage(ctx context.Context, tenantUUID string, tagUUIDs ...string) error {
	return r.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, tagUUID := range tagUUIDs {
			tagUUID = strings.TrimSpace(tagUUID)
			if tagUUID == "" {
				continue
			}
			var total int64
			if err := tx.Model(&model.TagBinding{}).Where("tenant_uuid = ? AND tag_uuid = ?", tenantUUID, tagUUID).Count(&total).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.Tag{}).Where("tenant_uuid = ? AND uuid = ?", tenantUUID, tagUUID).Update("usage_count", total).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *TagRepository) MergeTags(ctx context.Context, tenantUUID, sourceTagUUID, targetTagUUID string) (int64, error) {
	var moved int64
	err := r.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sourceBindings []model.TagBinding
		if err := tx.Where("tenant_uuid = ? AND tag_uuid = ?", tenantUUID, sourceTagUUID).Find(&sourceBindings).Error; err != nil {
			return err
		}
		for i := range sourceBindings {
			binding := sourceBindings[i]
			var existing model.TagBinding
			err := tx.Where("tenant_uuid = ? AND tag_uuid = ? AND resource_type = ? AND resource_uuid = ?", tenantUUID, targetTagUUID, binding.ResourceType, binding.ResourceUUID).First(&existing).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if err := tx.Model(&model.TagBinding{}).
					Where("tenant_uuid = ? AND tag_uuid = ? AND resource_type = ? AND resource_uuid = ?", tenantUUID, sourceTagUUID, binding.ResourceType, binding.ResourceUUID).
					Update("tag_uuid", targetTagUUID).Error; err != nil {
					return err
				}
				moved++
				continue
			}
			if err != nil {
				return err
			}
			if err := tx.Where("tenant_uuid = ? AND tag_uuid = ? AND resource_type = ? AND resource_uuid = ?", tenantUUID, sourceTagUUID, binding.ResourceType, binding.ResourceUUID).Delete(&model.TagBinding{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&model.Tag{}).Where("tenant_uuid = ? AND uuid = ?", tenantUUID, sourceTagUUID).Updates(map[string]any{"status": model.StatusArchived, "usage_count": 0}).Error; err != nil {
			return err
		}
		var targetTotal int64
		if err := tx.Model(&model.TagBinding{}).Where("tenant_uuid = ? AND tag_uuid = ?", tenantUUID, targetTagUUID).Count(&targetTotal).Error; err != nil {
			return err
		}
		return tx.Model(&model.Tag{}).Where("tenant_uuid = ? AND uuid = ?", tenantUUID, targetTagUUID).Update("usage_count", targetTotal).Error
	})
	return moved, err
}
