package knowledge

import (
	"time"

	"github.com/google/uuid"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// IAMSyncTask 记录空间创建时的 IAM 同步情况。
type IAMSyncTask struct {
	coremodel.PowerModel

	SpaceUUID   uuid.UUID  `gorm:"column:space_uuid;type:uuid;not null;index" json:"space_uuid"`
	Provider    string     `gorm:"column:provider;type:varchar(64);not null" json:"provider"`
	Status      string     `gorm:"column:status;type:varchar(32);not null;default:'pending';index" json:"status"`
	RetryCount  int        `gorm:"column:retry_count;type:int;not null;default:0" json:"retry_count"`
	LastError   string     `gorm:"column:last_error;type:text" json:"last_error,omitempty"`
	LockedAt    *time.Time `gorm:"column:locked_at" json:"locked_at,omitempty"`
	CompletedAt *time.Time `gorm:"column:completed_at" json:"completed_at,omitempty"`
}

func (IAMSyncTask) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableKnowledgeIAMSyncTasks
}

const (
	IAMSyncStatusPending   = "pending"
	IAMSyncStatusRunning   = "running"
	IAMSyncStatusSucceeded = "succeeded"
	IAMSyncStatusFailed    = "failed"
)
