package capability_registry

import (
	"time"

	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// CapabilitySyncJob 记录每次能力同步 Worker 的执行结果。
type CapabilitySyncJob struct {
	coremodel.PowerUUIDModel

	CapabilityID  string         `gorm:"column:capability_id;type:varchar(128);index:idx_capability_sync_job_capability" json:"capability_id,omitempty"`
	PluginID      string         `gorm:"column:plugin_id;type:varchar(128);not null;index:idx_capability_sync_job_plugin" json:"plugin_id"`
	PluginVersion string         `gorm:"column:plugin_version;type:varchar(64);not null" json:"plugin_version"`
	Status        string         `gorm:"column:status;type:varchar(32);not null;index:idx_capability_sync_job_status" json:"status"`
	HashBefore    string         `gorm:"column:hash_before;type:varchar(128)" json:"hash_before,omitempty"`
	HashAfter     string         `gorm:"column:hash_after;type:varchar(128)" json:"hash_after,omitempty"`
	ErrorSummary  string         `gorm:"column:error_summary;type:text" json:"error_summary,omitempty"`
	StartedAt     time.Time      `gorm:"column:started_at;not null" json:"started_at"`
	FinishedAt    *time.Time     `gorm:"column:finished_at" json:"finished_at,omitempty"`
	Metadata      datatypes.JSON `gorm:"column:metadata;type:jsonb;default:'{}'" json:"metadata,omitempty"`
}

func (CapabilitySyncJob) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableCapabilityRegistrySyncJob
}
