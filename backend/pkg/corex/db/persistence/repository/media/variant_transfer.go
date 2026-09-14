package media

import (
	"context"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"reflect"
)

// Parent-first locking serializes transfers with parent deletion and with each
// other. Only the declared upload state fields may be persisted by this path.
func (r *AssetRepository) WithVariantTransfer(ctx context.Context, tenant, asset, variant string, apply func(*m.MediaAsset, *m.MediaAssetVariant) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var parent m.MediaAsset
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_uuid = ? AND uuid = ?", tenant, asset).First(&parent).Error; err != nil {
			return err
		}
		var child m.MediaAssetVariant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_uuid = ? AND asset_uuid = ? AND uuid = ?", tenant, asset, variant).First(&child).Error; err != nil {
			return err
		}
		before := variantTransferFields(&child)
		if err := apply(&parent, &child); err != nil {
			return err
		}
		after := variantTransferFields(&child)
		if reflect.DeepEqual(before, after) {
			return nil
		}
		return tx.Model(&child).Where("tenant_uuid = ? AND asset_uuid = ? AND uuid = ?", tenant, asset, variant).Updates(after).Error
	})
}

func variantTransferFields(v *m.MediaAssetVariant) map[string]any {
	var expires, completed any
	if v.UploadExpiresAt != nil {
		expires = *v.UploadExpiresAt
	}
	if v.CompletedAt != nil {
		completed = *v.CompletedAt
	}
	return map[string]any{"upload_state": v.UploadState, "expected_checksum": v.ExpectedChecksum, "upload_expires_at": expires, "completed_at": completed, "ticket_version": v.TicketVersion}
}
