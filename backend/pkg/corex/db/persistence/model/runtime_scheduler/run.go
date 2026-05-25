package runtimescheduler

import (
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	TriggerSourceOnce     = "once"
	TriggerSourceInterval = "interval"
	TriggerSourceCron     = "cron"
	TriggerSourceManual   = "manual"
	TriggerSourceRetry    = "retry"

	RunStatusTriggered = "triggered"
	RunStatusSkipped   = "skipped"
	RunStatusFailed    = "failed"
)

type SchedulerJobRun struct {
	coremodel.PowerUUIDModel

	JobUUID         uuid.UUID  `gorm:"column:job_uuid;type:uuid;not null;index:idx_scheduler_job_runs_job_uuid" json:"job_uuid"`
	TenantUUID      string     `gorm:"column:tenant_uuid;type:char(36);not null;index:idx_scheduler_job_runs_owner,priority:1" json:"tenant_uuid"`
	OwnerType       string     `gorm:"column:owner_type;type:varchar(32);not null;index:idx_scheduler_job_runs_owner,priority:2" json:"owner_type"`
	OwnerID         string     `gorm:"column:owner_id;type:varchar(128);not null;index:idx_scheduler_job_runs_owner,priority:3" json:"owner_id"`
	TriggerSource   string     `gorm:"column:trigger_source;type:varchar(32);not null" json:"trigger_source"`
	ScheduledAt     *time.Time `gorm:"column:scheduled_at" json:"scheduled_at,omitempty"`
	FiredAt         *time.Time `gorm:"column:fired_at" json:"fired_at,omitempty"`
	Status          string     `gorm:"column:status;type:varchar(32);not null;index:idx_scheduler_job_runs_status" json:"status"`
	EventID         string     `gorm:"column:event_id;type:varchar(128);index:idx_scheduler_job_runs_event_id" json:"event_id,omitempty"`
	TraceID         string     `gorm:"column:trace_id;type:varchar(128);index:idx_scheduler_job_runs_trace_id" json:"trace_id"`
	ActorType       string     `gorm:"column:actor_type;type:varchar(32)" json:"actor_type,omitempty"`
	ActorUserID     uint64     `gorm:"column:actor_user_id;index:idx_scheduler_job_runs_actor_user" json:"actor_user_id,omitempty"`
	ActorUserUUID   string     `gorm:"column:actor_user_uuid;type:varchar(64)" json:"actor_user_uuid,omitempty"`
	ActorMemberID   uint64     `gorm:"column:actor_member_id;index:idx_scheduler_job_runs_actor_member" json:"actor_member_id,omitempty"`
	ActorMemberUUID string     `gorm:"column:actor_member_uuid;type:varchar(64)" json:"actor_member_uuid,omitempty"`
	ErrorCode       string     `gorm:"column:error_code;type:varchar(128)" json:"error_code,omitempty"`
	ErrorMessage    string     `gorm:"column:error_message;type:text" json:"error_message,omitempty"`
}

func (SchedulerJobRun) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableSchedulerJobRuns
}

func (m *SchedulerJobRun) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	m.TenantUUID = strings.TrimSpace(m.TenantUUID)
	m.OwnerType = strings.TrimSpace(m.OwnerType)
	m.OwnerID = strings.TrimSpace(m.OwnerID)
	if strings.TrimSpace(m.TriggerSource) == "" {
		m.TriggerSource = TriggerSourceManual
	}
	if strings.TrimSpace(m.Status) == "" {
		m.Status = RunStatusTriggered
	}
	return nil
}
