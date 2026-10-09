package ops

import (
	"strings"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

type BackupType string

const (
	BackupTypeLogical  BackupType = "logical"
	BackupTypePhysical BackupType = "physical"
	BackupTypeWAL      BackupType = "wal"
)

// BackupPolicy 定义备份策略与调度。
type BackupPolicy struct {
	coremodel.PowerUUIDModel

	Name              string     `gorm:"column:name;type:varchar(128);not null;index:idx_ops_backup_policy_name" json:"name"`
	BackupType        BackupType `gorm:"column:backup_type;type:varchar(32);not null;index:idx_ops_backup_policy_type" json:"backup_type"`
	Schedule          string     `gorm:"column:schedule;type:varchar(128);not null" json:"schedule"`
	RetentionDays     int32      `gorm:"column:retention_days;not null" json:"retention_days"`
	RetentionMode     string     `gorm:"column:retention_mode;type:varchar(32);not null;default:count" json:"retention_mode"`
	IntervalHours     int32      `gorm:"column:interval_hours;not null;default:1" json:"interval_hours"`
	RetentionCount    int32      `gorm:"column:retention_count;not null;default:168" json:"retention_count"`
	Timezone          string     `gorm:"column:timezone;type:varchar(64);not null;default:'Asia/Shanghai'" json:"timezone"`
	DrillEnabled      bool       `gorm:"column:drill_enabled;not null;default:false" json:"drill_enabled"`
	DrillIntervalDays int32      `gorm:"column:drill_interval_days;not null;default:7" json:"drill_interval_days"`
	TargetRef         string     `gorm:"column:target_ref;type:varchar(255);not null;default:''" json:"target_ref"`
	IsCurrent         bool       `gorm:"column:is_current;not null;default:false;index:idx_ops_backup_policy_current" json:"is_current"`
	Enabled           bool       `gorm:"column:enabled;not null;default:false;index:idx_ops_backup_policy_enabled" json:"enabled"`
	StorageTarget     string     `gorm:"column:storage_target;type:varchar(255);not null" json:"storage_target"`
	CreatedBy         string     `gorm:"column:created_by;type:varchar(128);not null" json:"created_by"`
	UpdatedBy         string     `gorm:"column:updated_by;type:varchar(128);not null" json:"updated_by"`
}

func (BackupPolicy) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableOpsBackupPolicies
}

func (m *BackupPolicy) Normalize() {
	m.Name = strings.TrimSpace(m.Name)
	m.BackupType = BackupType(strings.TrimSpace(strings.ToLower(string(m.BackupType))))
	m.Schedule = strings.TrimSpace(m.Schedule)
	m.Timezone = strings.TrimSpace(m.Timezone)
	m.TargetRef = strings.TrimSpace(m.TargetRef)
	m.StorageTarget = strings.TrimSpace(m.StorageTarget)
	m.CreatedBy = strings.TrimSpace(m.CreatedBy)
	m.UpdatedBy = strings.TrimSpace(m.UpdatedBy)
}
