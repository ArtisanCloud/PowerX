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

	Name          string     `gorm:"column:name;type:varchar(128);not null;index:idx_ops_backup_policy_name" json:"name"`
	BackupType    BackupType `gorm:"column:backup_type;type:varchar(32);not null;index:idx_ops_backup_policy_type" json:"backup_type"`
	Schedule      string     `gorm:"column:schedule;type:varchar(128);not null" json:"schedule"`
	RetentionDays int32      `gorm:"column:retention_days;not null" json:"retention_days"`
	Enabled       bool       `gorm:"column:enabled;not null;default:true;index:idx_ops_backup_policy_enabled" json:"enabled"`
	StorageTarget string     `gorm:"column:storage_target;type:varchar(255);not null" json:"storage_target"`
	CreatedBy     string     `gorm:"column:created_by;type:varchar(128);not null" json:"created_by"`
	UpdatedBy     string     `gorm:"column:updated_by;type:varchar(128);not null" json:"updated_by"`
}

func (BackupPolicy) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableOpsBackupPolicies
}

func (m *BackupPolicy) Normalize() {
	m.Name = strings.TrimSpace(m.Name)
	m.BackupType = BackupType(strings.TrimSpace(strings.ToLower(string(m.BackupType))))
	m.Schedule = strings.TrimSpace(m.Schedule)
	m.StorageTarget = strings.TrimSpace(m.StorageTarget)
	m.CreatedBy = strings.TrimSpace(m.CreatedBy)
	m.UpdatedBy = strings.TrimSpace(m.UpdatedBy)
}
