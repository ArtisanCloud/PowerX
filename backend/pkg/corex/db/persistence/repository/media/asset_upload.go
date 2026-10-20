package media

import (
	"context"
	"reflect"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// WithAssetUpload 将原文件的重试、写入和完成确认串行化；仅更新上传生命周期字段。
func (r *AssetRepository) WithAssetUpload(ctx context.Context, tenant, asset string, apply func(*m.MediaAsset) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row m.MediaAsset
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_uuid = ? AND uuid = ?", tenant, asset).First(&row).Error; err != nil {
			return err
		}
		before := assetUploadFields(&row)
		if err := apply(&row); err != nil {
			return err
		}
		after := assetUploadFields(&row)
		if reflect.DeepEqual(before, after) {
			return nil
		}
		return tx.Model(&row).Where("tenant_uuid = ? AND uuid = ?", tenant, asset).Updates(after).Error
	})
}

func assetUploadFields(row *m.MediaAsset) map[string]any {
	var expires any
	if row.UploadExpiresAt != nil {
		expires = *row.UploadExpiresAt
	}
	return map[string]any{"upload_state": row.UploadState, "upload_ticket_version": row.UploadTicketVersion, "upload_expires_at": expires, "meta": row.Meta, "updated_by": row.UpdatedBy}
}
