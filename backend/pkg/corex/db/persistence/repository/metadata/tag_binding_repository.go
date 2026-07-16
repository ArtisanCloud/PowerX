package metadata

import (
	"context"
	"strings"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TagBindingRepository struct {
	*Repository
}

func NewTagBindingRepository(db *gorm.DB) *TagBindingRepository {
	return &TagBindingRepository{Repository: NewRepository(db)}
}

func (r *TagBindingRepository) ListByResource(ctx context.Context, tenantUUID, resourceType, resourceUUID string) ([]model.TagBinding, []model.Tag, error) {
	var bindings []model.TagBinding
	if err := r.DB().WithContext(ctx).
		Where("tenant_uuid = ? AND resource_type = ? AND resource_uuid = ?", tenantUUID, resourceType, resourceUUID).
		Order("tag_uuid ASC").
		Find(&bindings).Error; err != nil {
		return nil, nil, err
	}
	tagUUIDs := make([]string, 0, len(bindings))
	for i := range bindings {
		tagUUIDs = append(tagUUIDs, bindings[i].TagUUID)
	}
	var tags []model.Tag
	if len(tagUUIDs) > 0 {
		if err := r.DB().WithContext(ctx).Where("tenant_uuid = ? AND uuid IN ?", tenantUUID, tagUUIDs).Find(&tags).Error; err != nil {
			return nil, nil, err
		}
	}
	return bindings, tags, nil
}

func (r *TagBindingRepository) ReplaceByResource(ctx context.Context, tenantUUID, resourceType, resourceUUID, createdByUUID string, tagUUIDs []string) ([]model.TagBinding, error) {
	out := make([]model.TagBinding, 0, len(tagUUIDs))
	err := r.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var oldBindings []model.TagBinding
		if err := tx.Where("tenant_uuid = ? AND resource_type = ? AND resource_uuid = ?", tenantUUID, resourceType, resourceUUID).Find(&oldBindings).Error; err != nil {
			return err
		}
		if err := tx.Where("tenant_uuid = ? AND resource_type = ? AND resource_uuid = ?", tenantUUID, resourceType, resourceUUID).Delete(&model.TagBinding{}).Error; err != nil {
			return err
		}
		for _, tagUUID := range tagUUIDs {
			tagUUID = strings.TrimSpace(tagUUID)
			if tagUUID == "" {
				continue
			}
			out = append(out, model.TagBinding{
				TenantUUID:    tenantUUID,
				TagUUID:       tagUUID,
				ResourceType:  resourceType,
				ResourceUUID:  resourceUUID,
				CreatedByUUID: createdByUUID,
			})
		}
		if len(out) > 0 {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&out).Error; err != nil {
				return err
			}
		}
		affected := map[string]struct{}{}
		for i := range oldBindings {
			affected[oldBindings[i].TagUUID] = struct{}{}
		}
		for i := range out {
			affected[out[i].TagUUID] = struct{}{}
		}
		for tagUUID := range affected {
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
	if err != nil {
		return nil, err
	}
	return out, nil
}
