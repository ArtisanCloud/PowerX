package storagegovmodel

import (
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// Sample 每实例每五分钟最多一份元数据；不保存路径、文件名或业务正文。
type Sample struct {
	coremodel.PowerModel
	TenantID      uint64         `gorm:"not null;default:0;uniqueIndex:uk_storage_sample_slot;index:idx_storage_sample_window"`
	InstanceID    string         `gorm:"type:varchar(128);not null;uniqueIndex:uk_storage_sample_slot;index:idx_storage_sample_window"`
	Slot          time.Time      `gorm:"not null;uniqueIndex:uk_storage_sample_slot;index:idx_storage_sample_window"`
	MetricVersion string         `gorm:"type:varchar(64);not null"`
	Document      datatypes.JSON `gorm:"type:jsonb;not null"`
	Status        int16          `gorm:"not null;default:1"`
}

func (*Sample) TableName() string { return coremodel.PowerXSchema + "." + TableSample }
