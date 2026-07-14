package metadata

import (
	"context"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ReferenceRepository struct {
	*Repository
}

func NewReferenceRepository(db *gorm.DB) *ReferenceRepository {
	return &ReferenceRepository{Repository: NewRepository(db)}
}

func (r *ReferenceRepository) Register(ctx context.Context, refs []model.Reference) error {
	if len(refs) == 0 {
		return nil
	}
	return r.DB().WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&refs).Error
}

func (r *ReferenceRepository) ReplaceForResource(ctx context.Context, tenantUUID, resourceType, resourceUUID string, refs []model.Reference) error {
	return r.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_uuid = ? AND resource_type = ? AND resource_uuid = ?", tenantUUID, resourceType, resourceUUID).Delete(&model.Reference{}).Error; err != nil {
			return err
		}
		if len(refs) == 0 {
			return nil
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&refs).Error
	})
}

func (r *ReferenceRepository) DeleteForResource(ctx context.Context, tenantUUID, resourceType, resourceUUID string) error {
	return r.DB().WithContext(ctx).Where("tenant_uuid = ? AND resource_type = ? AND resource_uuid = ?", tenantUUID, resourceType, resourceUUID).Delete(&model.Reference{}).Error
}

func (r *ReferenceRepository) ListForResource(ctx context.Context, tenantUUID, resourceType, resourceUUID string) ([]model.Reference, error) {
	var rows []model.Reference
	err := r.DB().WithContext(ctx).
		Where("tenant_uuid = ? AND resource_type = ? AND resource_uuid = ?", tenantUUID, resourceType, resourceUUID).
		Order("metadata_type ASC, metadata_uuid ASC, field_name ASC").
		Find(&rows).Error
	return rows, err
}

func (r *ReferenceRepository) CountForMetadata(ctx context.Context, tenantUUID, metadataType, metadataUUID string) (int64, error) {
	var total int64
	err := r.DB().WithContext(ctx).
		Model(&model.Reference{}).
		Where("tenant_uuid = ? AND metadata_type = ? AND metadata_uuid = ?", tenantUUID, metadataType, metadataUUID).
		Count(&total).Error
	return total, err
}
