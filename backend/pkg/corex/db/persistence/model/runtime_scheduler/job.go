package runtimescheduler

import (
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	OwnerTypeCore   = "core"
	OwnerTypePlugin = "plugin"

	ScheduleTypeOnce     = "once"
	ScheduleTypeInterval = "interval"
	ScheduleTypeCron     = "cron"

	JobStatusActive    = "active"
	JobStatusPaused    = "paused"
	JobStatusCompleted = "completed"
	JobStatusDeleted   = "deleted"

	MisfirePolicySkip       = "skip"
	MisfirePolicyRunCatchup = "run_catchup"

	OverlapPolicySkip     = "skip"
	OverlapPolicyQueue    = "queue"
	OverlapPolicyParallel = "parallel"

	TopicSchedulerTriggeredV1 = "powerx.runtime.scheduler.triggered.v1"
)

type SchedulerJob struct {
	coremodel.PowerUUIDModel

	TenantUUID     string         `gorm:"column:tenant_uuid;type:char(36);not null;uniqueIndex:uk_scheduler_job_owner_name,priority:1;index:idx_scheduler_jobs_due,priority:1" json:"tenant_uuid"`
	OwnerType      string         `gorm:"column:owner_type;type:varchar(32);not null;uniqueIndex:uk_scheduler_job_owner_name,priority:2;index:idx_scheduler_jobs_due,priority:2" json:"owner_type"`
	OwnerID        string         `gorm:"column:owner_id;type:varchar(128);not null;uniqueIndex:uk_scheduler_job_owner_name,priority:3;index:idx_scheduler_jobs_due,priority:3" json:"owner_id"`
	Name           string         `gorm:"column:name;type:varchar(160);not null;uniqueIndex:uk_scheduler_job_owner_name,priority:4" json:"name"`
	ScheduleType   string         `gorm:"column:schedule_type;type:varchar(32);not null" json:"schedule_type"`
	ScheduleExpr   string         `gorm:"column:schedule_expr;type:varchar(255);not null" json:"schedule_expr"`
	Timezone       string         `gorm:"column:timezone;type:varchar(64);not null;default:'UTC'" json:"timezone"`
	Topic          string         `gorm:"column:topic;type:varchar(160);not null;default:'powerx.runtime.scheduler.triggered.v1'" json:"topic"`
	PayloadJSON    datatypes.JSON `gorm:"column:payload_json;type:jsonb;not null;default:'{}'" json:"payload_json"`
	Status         string         `gorm:"column:status;type:varchar(32);not null;default:'active';index:idx_scheduler_jobs_due,priority:4" json:"status"`
	NextRunAt      *time.Time     `gorm:"column:next_run_at;index:idx_scheduler_jobs_due,priority:5" json:"next_run_at,omitempty"`
	LastRunAt      *time.Time     `gorm:"column:last_run_at" json:"last_run_at,omitempty"`
	MisfirePolicy  string         `gorm:"column:misfire_policy;type:varchar(32);not null;default:'skip'" json:"misfire_policy"`
	OverlapPolicy  string         `gorm:"column:overlap_policy;type:varchar(32);not null;default:'skip'" json:"overlap_policy"`
	RetryPolicy    datatypes.JSON `gorm:"column:retry_policy_json;type:jsonb" json:"retry_policy_json,omitempty"`
	IdempotencyKey string         `gorm:"column:idempotency_key;type:varchar(255);index:idx_scheduler_jobs_idempotency_key" json:"idempotency_key,omitempty"`
	CreatedBy      string         `gorm:"column:created_by;type:varchar(128)" json:"created_by,omitempty"`
	UpdatedBy      string         `gorm:"column:updated_by;type:varchar(128)" json:"updated_by,omitempty"`
	LastError      string         `gorm:"column:last_error;type:text" json:"last_error,omitempty"`
	TraceID        string         `gorm:"column:trace_id;type:varchar(128);index:idx_scheduler_jobs_trace_id" json:"trace_id,omitempty"`
}

func (SchedulerJob) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableSchedulerJobs
}

func (m *SchedulerJob) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	m.TenantUUID = strings.TrimSpace(m.TenantUUID)
	m.OwnerType = strings.TrimSpace(m.OwnerType)
	m.OwnerID = strings.TrimSpace(m.OwnerID)
	m.Name = strings.TrimSpace(m.Name)
	m.ScheduleType = strings.TrimSpace(m.ScheduleType)
	m.ScheduleExpr = strings.TrimSpace(m.ScheduleExpr)
	m.Timezone = strings.TrimSpace(m.Timezone)
	if m.Timezone == "" {
		m.Timezone = "UTC"
	}
	if strings.TrimSpace(m.Topic) == "" {
		m.Topic = TopicSchedulerTriggeredV1
	}
	if strings.TrimSpace(m.Status) == "" {
		m.Status = JobStatusActive
	}
	if strings.TrimSpace(m.MisfirePolicy) == "" {
		m.MisfirePolicy = MisfirePolicySkip
	}
	if strings.TrimSpace(m.OverlapPolicy) == "" {
		m.OverlapPolicy = OverlapPolicySkip
	}
	if len(m.PayloadJSON) == 0 {
		m.PayloadJSON = datatypes.JSON([]byte("{}"))
	}
	return nil
}
