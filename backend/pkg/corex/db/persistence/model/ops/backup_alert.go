package ops

import (
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

type BackupAlertLevel string

const (
	BackupAlertLevelLow    BackupAlertLevel = "low"
	BackupAlertLevelMedium BackupAlertLevel = "medium"
	BackupAlertLevelHigh   BackupAlertLevel = "high"
)

// BackupAlert 记录备份任务相关的告警事件。
type BackupAlert struct {
	coremodel.PowerUUIDModel

	PolicyID     uint64           `gorm:"column:policy_id;not null;index:idx_ops_backup_alert_policy" json:"policy_id"`
	JobID        uint64           `gorm:"column:job_id;not null;index:idx_ops_backup_alert_job" json:"job_id"`
	Level        BackupAlertLevel `gorm:"column:level;type:varchar(16);not null;index:idx_ops_backup_alert_level" json:"level"`
	AlertType    string           `gorm:"column:alert_type;type:varchar(64);not null;index:idx_ops_backup_alert_type" json:"alert_type"`
	Message      string           `gorm:"column:message;type:text;not null" json:"message"`
	Suggestion   string           `gorm:"column:suggestion;type:text" json:"suggestion,omitempty"`
	Acknowledged bool             `gorm:"column:acknowledged;not null;default:false;index:idx_ops_backup_alert_ack" json:"acknowledged"`
	AckBy        string           `gorm:"column:ack_by;type:varchar(128)" json:"ack_by,omitempty"`
	AckAt        *time.Time       `gorm:"column:ack_at" json:"ack_at,omitempty"`
	TraceID      string           `gorm:"column:trace_id;type:varchar(128);index:idx_ops_backup_alert_trace" json:"trace_id,omitempty"`
}

func (BackupAlert) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableOpsBackupAlerts
}

func (m *BackupAlert) Normalize() {
	m.Level = BackupAlertLevel(strings.TrimSpace(strings.ToLower(string(m.Level))))
	m.AlertType = strings.TrimSpace(strings.ToLower(m.AlertType))
	m.Message = strings.TrimSpace(m.Message)
	m.Suggestion = strings.TrimSpace(m.Suggestion)
	m.AckBy = strings.TrimSpace(m.AckBy)
	m.TraceID = strings.TrimSpace(m.TraceID)
}
