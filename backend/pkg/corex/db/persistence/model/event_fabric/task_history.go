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
	TaskHistoryStatusPending   = "pending"
	TaskHistoryStatusDeferred  = "deferred"
	TaskHistoryStatusRunning   = "running"
	TaskHistoryStatusCompleted = "completed"
	TaskHistoryStatusFailed    = "failed"
	TaskHistoryStatusCancelled = "cancelled"
)

type TaskHistory struct {
	coremodel.PowerUUIDModel

	TaskID       string         `gorm:"column:task_id;type:varchar(191);not null;uniqueIndex:uidx_event_task_histories_key" json:"task_id"`
	TenantKey    string         `gorm:"column:tenant_key;type:varchar(128);not null;uniqueIndex:uidx_event_task_histories_key;index:idx_event_task_histories_tenant_subscriber" json:"tenant_key"`
	SubscriberID string         `gorm:"column:subscriber_id;type:varchar(191);not null;uniqueIndex:uidx_event_task_histories_key;index:idx_event_task_histories_tenant_subscriber" json:"subscriber_id"`
	Topic        string         `gorm:"column:topic;type:varchar(255);not null;default:'';index:idx_event_task_histories_topic" json:"topic"`
	Kind         string         `gorm:"column:kind;type:varchar(128);not null;default:'';index:idx_event_task_histories_kind" json:"kind"`
	Source       string         `gorm:"column:source;type:varchar(64);not null;default:'';index:idx_event_task_histories_source" json:"source"`
	TraceID      string         `gorm:"column:trace_id;type:varchar(128);not null;default:'';index:idx_event_task_histories_trace_id" json:"trace_id"`
	Status       string         `gorm:"column:status;type:varchar(32);not null;default:'pending';index:idx_event_task_histories_status" json:"status"`
	Attempt      int            `gorm:"column:attempt;not null;default:0" json:"attempt"`
	ErrorMessage string         `gorm:"column:error_message;type:text" json:"error_message"`
	Payload      string         `gorm:"column:payload;type:text" json:"payload"`
	Metadata     datatypes.JSON `gorm:"column:metadata;type:jsonb;not null;default:'{}'" json:"metadata"`
	SubmittedAt  *time.Time     `gorm:"column:submitted_at;type:timestamp with time zone;index:idx_event_task_histories_submitted_at" json:"submitted_at"`
	StartedAt    *time.Time     `gorm:"column:started_at;type:timestamp with time zone" json:"started_at"`
	CompletedAt  *time.Time     `gorm:"column:completed_at;type:timestamp with time zone" json:"completed_at"`
	LastSeenAt   time.Time      `gorm:"column:last_seen_at;type:timestamp with time zone;not null;index:idx_event_task_histories_last_seen_at" json:"last_seen_at"`
}

func (TaskHistory) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableEventTaskHistories
}

func (m *TaskHistory) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	m.TaskID = strings.TrimSpace(m.TaskID)
	m.TenantKey = strings.TrimSpace(m.TenantKey)
	m.SubscriberID = strings.TrimSpace(m.SubscriberID)
	m.Topic = strings.TrimSpace(m.Topic)
	m.Kind = strings.TrimSpace(m.Kind)
	m.Source = strings.TrimSpace(m.Source)
	m.TraceID = strings.TrimSpace(m.TraceID)
	if strings.TrimSpace(m.Status) == "" {
		m.Status = TaskHistoryStatusPending
	}
	if m.LastSeenAt.IsZero() {
		m.LastSeenAt = time.Now().UTC()
	}
	return nil
}
