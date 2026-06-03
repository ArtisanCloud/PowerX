package media

import (
	"time"

	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

type MediaAssetVariant struct {
	coremodel.PowerUUIDModel

	AuditMixin

	TenantUUID string `gorm:"column:tenant_uuid;type:char(36);not null;index:idx_media_asset_variant_tenant_asset,priority:1;uniqueIndex:uk_media_asset_variant_tenant_asset_name,priority:1" json:"tenant_uuid"`
	AssetUUID  string `gorm:"column:asset_uuid;type:char(36);not null;index:idx_media_asset_variant_tenant_asset,priority:2;uniqueIndex:uk_media_asset_variant_tenant_asset_name,priority:2" json:"asset_uuid"`
	Variant    string `gorm:"column:variant;type:varchar(32);not null;uniqueIndex:uk_media_asset_variant_tenant_asset_name,priority:3" json:"variant"`
	Name       string `gorm:"column:name;type:varchar(255);not null;default:''" json:"name"`
	Driver     string `gorm:"column:driver;type:varchar(32);not null;index;uniqueIndex:uk_media_asset_variant_driver_key,priority:1" json:"driver"`
	StorageKey string `gorm:"column:storage_key;type:varchar(1024);not null;uniqueIndex:uk_media_asset_variant_driver_key,priority:2" json:"storage_key"`
	Bucket     string `gorm:"column:bucket;type:varchar(128);not null;default:''" json:"bucket"`
	BaseURL    string `gorm:"column:base_url;type:varchar(512);not null;default:''" json:"base_url"`

	SizeBytes int64  `gorm:"column:size_bytes;not null;default:0" json:"size_bytes"`
	MimeType  string `gorm:"column:mime_type;type:varchar(128);not null;default:''" json:"mime_type"`

	Meta                    datatypes.JSON `gorm:"column:meta;type:jsonb;not null;default:'{}'::jsonb" json:"meta,omitempty"`
	LastPresignedAt         *time.Time     `gorm:"column:last_presigned_at" json:"last_presigned_at,omitempty"`
	LastPresignedTTLSeconds int32          `gorm:"column:last_presigned_ttl_seconds;not null;default:43200" json:"last_presigned_ttl_seconds"`
}

func (m *MediaAssetVariant) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableMediaAssetVariant
}

func (m *MediaAssetVariant) GetTableName(needFull bool) string {
	if needFull {
		return m.TableName()
	}
	return coremodel.TableMediaAssetVariant
}
