package eventfabric

import (
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	ScheduledTaskRunTriggerSchedule = "schedule"
	ScheduledTaskRunTriggerManual   = "manual"

	ScheduledTaskRunStatusPending   = "pending"
	ScheduledTaskRunStatusRunning   = "running"
	ScheduledTaskRunStatusSucceeded = "succeeded"
	ScheduledTaskRunStatusFailed    = "failed"
	ScheduledTaskRunStatusCancelled = "cancelled"
)

// ScheduledTaskRun 记录 Cron 任务每次触发的执行过程。
type ScheduledTaskRun struct {
	coremodel.PowerUUIDModel

	TenantUUID         string         `gorm:"column:tenant_uuid;type:char(36);not null;default:'';index:idx_event_scheduled_task_runs_tenant_uuid" json:"tenant_uuid"`
	ScheduledTaskUUID  uuid.UUID      `gorm:"column:scheduled_task_uuid;type:uuid;not null;index:idx_event_scheduled_task_runs_task_uuid" json:"scheduled_task_uuid"`
	TriggerType        string         `gorm:"column:trigger_type;type:varchar(32);not null;default:'schedule'" json:"trigger_type"`
	Status             string         `gorm:"column:status;type:varchar(32);not null;default:'pending';index:idx_event_scheduled_task_runs_status" json:"status"`
	Attempt            int            `gorm:"column:attempt;not null;default:1" json:"attempt"`
	StartedAt          *time.Time     `gorm:"column:started_at;type:timestamp with time zone;index:idx_event_scheduled_task_runs_started_at" json:"started_at,omitempty"`
	FinishedAt         *time.Time     `gorm:"column:finished_at;type:timestamp with time zone" json:"finished_at,omitempty"`
	ErrorMessage       string         `gorm:"column:error_message;type:text" json:"error_message,omitempty"`
	TraceID            string         `gorm:"column:trace_id;type:varchar(128);index:idx_event_scheduled_task_runs_trace_id" json:"trace_id,omitempty"`
	EventID            string         `gorm:"column:event_id;type:varchar(128);index:idx_event_scheduled_task_runs_event_id" json:"event_id,omitempty"`
	PayloadSnapshot    datatypes.JSON `gorm:"column:payload_snapshot;type:jsonb;default:'{}'" json:"payload_snapshot,omitempty"`
	ExecutionMetadata  datatypes.JSON `gorm:"column:execution_metadata;type:jsonb;default:'{}'" json:"execution_metadata,omitempty"`
	FailureRetryable   bool           `gorm:"column:failure_retryable;not null;default:false" json:"failure_retryable"`
	FailureCategory    string         `gorm:"column:failure_category;type:varchar(64)" json:"failure_category,omitempty"`
	DispatchedAt       *time.Time     `gorm:"column:dispatched_at;type:timestamp with time zone" json:"dispatched_at,omitempty"`
	CompletedEventUUID *uuid.UUID     `gorm:"column:completed_event_uuid;type:uuid" json:"completed_event_uuid,omitempty"`
}

func (ScheduledTaskRun) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableEventScheduledTaskRuns
}

func (m *ScheduledTaskRun) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	m.TenantUUID = strings.TrimSpace(m.TenantUUID)
	if m.TriggerType == "" {
		m.TriggerType = ScheduledTaskRunTriggerSchedule
	}
	if m.Status == "" {
		m.Status = ScheduledTaskRunStatusPending
	}
	if m.Attempt <= 0 {
		m.Attempt = 1
	}
	return nil
}
