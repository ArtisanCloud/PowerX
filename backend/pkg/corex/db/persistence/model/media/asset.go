package media

import (
	"time"

	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

type AuditMixin struct {
	CreatedBy *uint64 `gorm:"column:created_by" json:"created_by,omitempty"`
	UpdatedBy *uint64 `gorm:"column:updated_by" json:"updated_by,omitempty"`
	DeletedBy *uint64 `gorm:"column:deleted_by" json:"deleted_by,omitempty"`
}

type MediaAsset struct {
	coremodel.PowerUUIDModel

	AuditMixin

	TenantUUID string `gorm:"column:tenant_uuid;type:char(36);not null;index:idx_media_asset_tenant_status,priority:1;index:idx_media_asset_owner,priority:1;uniqueIndex:uk_media_asset_tenant_driver_key,priority:1" json:"tenant_uuid"`
	Name       string `gorm:"column:name;type:varchar(255);not null;index" json:"name"`
	Driver     string `gorm:"column:driver;type:varchar(32);not null;index;uniqueIndex:uk_media_asset_tenant_driver_key,priority:2" json:"driver"`
	StorageKey string `gorm:"column:storage_key;type:varchar(1024);not null;uniqueIndex:uk_media_asset_tenant_driver_key,priority:3" json:"storage_key"`
	Bucket     string `gorm:"column:bucket;type:varchar(128);not null;default:''" json:"bucket"`
	BaseURL    string `gorm:"column:base_url;type:varchar(512);not null;default:''" json:"base_url"`

	SizeBytes int64  `gorm:"column:size_bytes;not null;default:0" json:"size_bytes"`
	MimeType  string `gorm:"column:mime_type;type:varchar(128);not null;default:''" json:"mime_type"`

	OwnerType string `gorm:"column:owner_type;type:varchar(32);not null;default:'';index:idx_media_asset_owner,priority:2" json:"owner_type"`
	OwnerID   string `gorm:"column:owner_id;type:varchar(64);not null;default:'';index:idx_media_asset_owner,priority:3" json:"owner_id"`

	BusinessStatus string         `gorm:"column:business_status;type:varchar(32);not null;default:'draft';index:idx_media_asset_tenant_status,priority:2" json:"business_status"`
	Tags           datatypes.JSON `gorm:"column:tags;type:jsonb;not null;default:'[]'::jsonb;index:idx_media_asset_tags_gin,type:gin" json:"tags,omitempty"`
	Meta           datatypes.JSON `gorm:"column:meta;type:jsonb;not null;default:'{}'::jsonb" json:"meta,omitempty"`

	LastPresignedAt         *time.Time `gorm:"column:last_presigned_at" json:"last_presigned_at,omitempty"`
	LastPresignedTTLSeconds int32      `gorm:"column:last_presigned_ttl_seconds;not null;default:43200" json:"last_presigned_ttl_seconds"`
}

func (m *MediaAsset) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableMediaAsset
}

func (m *MediaAsset) GetTableName(needFull bool) string {
	if needFull {
		return m.TableName()
	}
	return coremodel.TableMediaAsset
}
