package ops

import (
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

type MigrationStatus string
type MigrationStepStatus string

const (
	MigrationStatusPending MigrationStatus = "pending"
	MigrationStatusRunning MigrationStatus = "running"
	MigrationStatusSuccess MigrationStatus = "success"
	MigrationStatusFailed  MigrationStatus = "failed"

	MigrationStepPending MigrationStepStatus = "pending"
	MigrationStepSuccess MigrationStepStatus = "success"
	MigrationStepFailed  MigrationStepStatus = "failed"
)

// MigrationRunbookRecord 记录一次 A->B 实例迁移执行与验收结果。
type MigrationRunbookRecord struct {
	coremodel.PowerUUIDModel

	SourceEnv                string              `gorm:"column:source_env;type:varchar(64);not null;index:idx_ops_migration_source_target" json:"source_env"`
	TargetEnv                string              `gorm:"column:target_env;type:varchar(64);not null;index:idx_ops_migration_source_target" json:"target_env"`
	Status                   MigrationStatus     `gorm:"column:status;type:varchar(32);not null;index:idx_ops_migration_status" json:"status"`
	DBMigrationStatus        MigrationStepStatus `gorm:"column:db_migration_status;type:varchar(32);not null" json:"db_migration_status"`
	InstanceAcceptanceStatus MigrationStepStatus `gorm:"column:instance_acceptance_status;type:varchar(32);not null" json:"instance_acceptance_status"`
	TrafficSwitchStatus      MigrationStepStatus `gorm:"column:traffic_switch_status;type:varchar(32);not null" json:"traffic_switch_status"`
	TrafficRollbackStatus    MigrationStepStatus `gorm:"column:traffic_rollback_status;type:varchar(32);not null" json:"traffic_rollback_status"`
	DryRun                   bool                `gorm:"column:dry_run;not null;default:false" json:"dry_run"`
	Summary                  string              `gorm:"column:summary;type:text" json:"summary,omitempty"`
	Operator                 string              `gorm:"column:operator;type:varchar(128);not null" json:"operator"`
	TraceID                  string              `gorm:"column:trace_id;type:varchar(128);index:idx_ops_migration_trace" json:"trace_id,omitempty"`
	StartedAt                *time.Time          `gorm:"column:started_at" json:"started_at,omitempty"`
	EndedAt                  *time.Time          `gorm:"column:ended_at" json:"ended_at,omitempty"`
	ErrorMessage             string              `gorm:"column:error_message;type:text" json:"error_message,omitempty"`
}

func (MigrationRunbookRecord) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableOpsMigrationRunbookRecords
}

func (m *MigrationRunbookRecord) Normalize() {
	m.SourceEnv = strings.TrimSpace(strings.ToLower(m.SourceEnv))
	m.TargetEnv = strings.TrimSpace(strings.ToLower(m.TargetEnv))
	m.Status = MigrationStatus(strings.TrimSpace(strings.ToLower(string(m.Status))))
	m.DBMigrationStatus = MigrationStepStatus(strings.TrimSpace(strings.ToLower(string(m.DBMigrationStatus))))
	m.InstanceAcceptanceStatus = MigrationStepStatus(strings.TrimSpace(strings.ToLower(string(m.InstanceAcceptanceStatus))))
	m.TrafficSwitchStatus = MigrationStepStatus(strings.TrimSpace(strings.ToLower(string(m.TrafficSwitchStatus))))
	m.TrafficRollbackStatus = MigrationStepStatus(strings.TrimSpace(strings.ToLower(string(m.TrafficRollbackStatus))))
	m.Summary = strings.TrimSpace(m.Summary)
	m.Operator = strings.TrimSpace(m.Operator)
	m.TraceID = strings.TrimSpace(m.TraceID)
	m.ErrorMessage = strings.TrimSpace(m.ErrorMessage)
}
