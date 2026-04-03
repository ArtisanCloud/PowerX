package ops

import (
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

type BackupJobStatus string

type BackupTriggerType string

const (
	BackupJobStatusPending BackupJobStatus = "pending"
	BackupJobStatusRunning BackupJobStatus = "running"
	BackupJobStatusSuccess BackupJobStatus = "success"
	BackupJobStatusFailed  BackupJobStatus = "failed"

	BackupTriggerTypeManual    BackupTriggerType = "manual"
	BackupTriggerTypeScheduled BackupTriggerType = "scheduled"
)

// BackupJob 记录备份任务执行。
type BackupJob struct {
	coremodel.PowerUUIDModel

	PolicyID     uint64            `gorm:"column:policy_id;not null;index:idx_ops_backup_job_policy" json:"policy_id"`
	Status       BackupJobStatus   `gorm:"column:status;type:varchar(32);not null;index:idx_ops_backup_job_status" json:"status"`
	TriggerType  BackupTriggerType `gorm:"column:trigger_type;type:varchar(32);not null" json:"trigger_type"`
	StartedAt    *time.Time        `gorm:"column:started_at" json:"started_at,omitempty"`
	EndedAt      *time.Time        `gorm:"column:ended_at" json:"ended_at,omitempty"`
	ErrorMessage string            `gorm:"column:error_message;type:text" json:"error_message,omitempty"`
	Operator     string            `gorm:"column:operator;type:varchar(128);not null" json:"operator"`
	TraceID      string            `gorm:"column:trace_id;type:varchar(128);index:idx_ops_backup_job_trace" json:"trace_id,omitempty"`
}

func (BackupJob) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableOpsBackupJobs
}

func (m *BackupJob) Normalize() {
	m.Status = BackupJobStatus(strings.TrimSpace(strings.ToLower(string(m.Status))))
	m.TriggerType = BackupTriggerType(strings.TrimSpace(strings.ToLower(string(m.TriggerType))))
	m.Operator = strings.TrimSpace(m.Operator)
	m.TraceID = strings.TrimSpace(m.TraceID)
}
