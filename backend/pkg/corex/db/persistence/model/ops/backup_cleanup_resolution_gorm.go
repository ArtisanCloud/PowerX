package ops

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"time"
)

// BackupCleanupResolution 单独审计当前人工保留／缺损隔离处置，不改写原未知请求。
// 平台级运维表以安装 scope 隔离，仅 Root 人类会话使用。
type BackupCleanupResolution struct {
	coremodel.PowerUUIDModel
	Operation         string    `gorm:"type:varchar(32);not null;default:retain_cleanup"`
	ExpectedDiagnosis string    `gorm:"type:varchar(32);not null;default:''"`
	DiagnosticDigest  string    `gorm:"type:varchar(64);not null;default:''"`
	RequestID         uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uk_backup_resolution_request"`
	ArtifactID        uint64    `gorm:"not null;index"`
	JobID             uint64    `gorm:"not null"`
	PolicyID          uint64    `gorm:"not null"`
	CleanupToken      string    `gorm:"type:varchar(36);not null;index"`
	ActorID           uint64    `gorm:"not null"`
	InputDigest       string    `gorm:"type:varchar(64);not null"`
	ReviewVersion     string    `gorm:"type:varchar(64);not null"`
	HistoryDigest     string    `gorm:"type:varchar(64);not null"`
	ScopeFingerprint  string    `gorm:"type:varchar(64);not null"`
	Generation        string    `gorm:"type:varchar(64);not null"`
	Fence             string    `gorm:"type:varchar(64);not null"`
	Reason            string    `gorm:"type:varchar(1024);not null"`
	State             string    `gorm:"type:varchar(24);not null"`
	Problem           string    `gorm:"type:varchar(96);not null;default:''"`
	CompletedAt       *time.Time
}

func (BackupCleanupResolution) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableOpsBackupCleanupResolutions
}
