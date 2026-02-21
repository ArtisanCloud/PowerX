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
	ScheduledTaskKindCron = "cron"

	ScheduledTaskStatusEnabled = "enabled"
	ScheduledTaskStatusPaused  = "paused"
	ScheduledTaskStatusRemoved = "removed"

	ScheduledTaskMisfireSkip    = "skip"
	ScheduledTaskMisfireFireNow = "fire_now"
	ScheduledTaskMisfireCatchUp = "catch_up"
)

// ScheduledTask 定义统一 Event Fabric Cron 任务。
type ScheduledTask struct {
	coremodel.PowerUUIDModel

	TenantUUID      string         `gorm:"column:tenant_uuid;type:char(36);not null;default:'';index:idx_event_scheduled_tasks_tenant_uuid;uniqueIndex:uk_event_scheduled_task_job_key,priority:1" json:"tenant_uuid"`
	JobKey          string         `gorm:"column:job_key;type:varchar(128);not null;uniqueIndex:uk_event_scheduled_task_job_key,priority:2" json:"job_key"`
	Name            string         `gorm:"column:name;type:varchar(128);not null" json:"name"`
	Description     string         `gorm:"column:description;type:text" json:"description,omitempty"`
	Status          string         `gorm:"column:status;type:varchar(32);not null;default:'enabled';index:idx_event_scheduled_tasks_status" json:"status"`
	Kind            string         `gorm:"column:kind;type:varchar(32);not null;default:'cron'" json:"kind"`
	CronExpr        string         `gorm:"column:cron_expr;type:varchar(128);not null" json:"cron_expr"`
	Timezone        string         `gorm:"column:timezone;type:varchar(64);not null;default:'UTC'" json:"timezone"`
	MisfirePolicy   string         `gorm:"column:misfire_policy;type:varchar(32);not null;default:'skip'" json:"misfire_policy"`
	Payload         datatypes.JSON `gorm:"column:payload;type:jsonb;default:'{}'" json:"payload,omitempty"`
	MaxRetry        int            `gorm:"column:max_retry;not null;default:5" json:"max_retry"`
	RetryBackoffSec int            `gorm:"column:retry_backoff_sec;not null;default:30" json:"retry_backoff_sec"`
	NextRunAt       *time.Time     `gorm:"column:next_run_at;type:timestamp with time zone;index:idx_event_scheduled_tasks_next_run_at" json:"next_run_at,omitempty"`
	LastRunAt       *time.Time     `gorm:"column:last_run_at;type:timestamp with time zone" json:"last_run_at,omitempty"`
	LastError       string         `gorm:"column:last_error;type:text" json:"last_error,omitempty"`
	CreatedBy       string         `gorm:"column:created_by;type:varchar(128)" json:"created_by,omitempty"`
	UpdatedBy       string         `gorm:"column:updated_by;type:varchar(128)" json:"updated_by,omitempty"`
	TraceID         string         `gorm:"column:trace_id;type:varchar(128);index:idx_event_scheduled_tasks_trace_id" json:"trace_id,omitempty"`
}

func (ScheduledTask) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableEventScheduledTasks
}

func (m *ScheduledTask) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	m.TenantUUID = strings.TrimSpace(m.TenantUUID)
	m.JobKey = strings.TrimSpace(m.JobKey)
	if m.Status == "" {
		m.Status = ScheduledTaskStatusEnabled
	}
	if m.Kind == "" {
		m.Kind = ScheduledTaskKindCron
	}
	if strings.TrimSpace(m.Timezone) == "" {
		m.Timezone = "UTC"
	}
	if m.MisfirePolicy == "" {
		m.MisfirePolicy = ScheduledTaskMisfireSkip
	}
	if m.MaxRetry <= 0 {
		m.MaxRetry = 5
	}
	if m.RetryBackoffSec <= 0 {
		m.RetryBackoffSec = 30
	}
	return nil
}
