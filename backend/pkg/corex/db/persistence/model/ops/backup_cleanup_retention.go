package ops

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"time"
)

// BackupCleanupRetention is platform-only operational audit, never an editable policy or deletion proof.
// Admitted identity/input fields are immutable; only checking -> retained/rejected is permitted.
type BackupCleanupRetention struct {
	coremodel.PowerUUIDModel
	RequestID        uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uk_backup_retain_request"`
	ArtifactID       uint64    `gorm:"not null;index;uniqueIndex:uk_backup_retain_active,priority:1,where:state = 'checking'"`
	CleanupToken     string    `gorm:"type:varchar(36);not null;uniqueIndex:uk_backup_retain_active,priority:2,where:state = 'checking'"`
	ActorID          uint64    `gorm:"not null"`
	InputDigest      string    `gorm:"type:varchar(64);not null"`
	ReviewVersion    string    `gorm:"type:varchar(64);not null"`
	ScopeFingerprint string    `gorm:"type:varchar(64);not null"`
	Fence            string    `gorm:"type:varchar(64);not null"`
	PolicyID         uint64    `gorm:"not null"`
	JobID            uint64    `gorm:"not null"`
	Reason           string    `gorm:"type:varchar(1024);not null"`
	State            string    `gorm:"type:varchar(24);not null"`
	Problem          string    `gorm:"type:varchar(96);not null;default:''"`
	CompletedAt      *time.Time
}

func (BackupCleanupRetention) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableOpsBackupCleanupRetentions
}
