package storagegovmodel

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
	"time"
)

// CleanupRun 保存预览、逐项删除意图与结果；未知结果不能重试成第二轮删除。
type CleanupRun struct {
	coremodel.PowerModel
	TenantID   uint64         `gorm:"not null;default:0;uniqueIndex:uk_storage_cleanup_run;index:idx_storage_cleanup_instance"`
	InstanceID string         `gorm:"type:varchar(128);not null;uniqueIndex:uk_storage_cleanup_run;index:idx_storage_cleanup_instance"`
	RunID      string         `gorm:"type:varchar(36);not null;uniqueIndex:uk_storage_cleanup_run"`
	Version    int64          `gorm:"not null"`
	State      string         `gorm:"type:varchar(32);not null;index"`
	ActorID    uint64         `gorm:"not null"`
	ExpiresAt  time.Time      `gorm:"not null"`
	Document   datatypes.JSON `gorm:"type:jsonb;not null"`
	Status     int16          `gorm:"not null;default:1"`
}

func (*CleanupRun) TableName() string { return coremodel.PowerXSchema + "." + TableCleanupRun }
