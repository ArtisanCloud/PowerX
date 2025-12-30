package eventfabric

import (
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// DlqMessage 记录已进入死信队列的事件。
type DlqMessage struct {
	coremodel.PowerUUIDModel

	TenantKey       string         `gorm:"column:tenant_key;type:varchar(128);not null;index:idx_event_dlq_tenant_key" json:"tenant_key"`
	TopicUUID       uuid.UUID      `gorm:"column:topic_uuid;type:uuid;not null;index:idx_event_dlq_topic" json:"topic_uuid"`
	EnvelopeUUID    uuid.UUID      `gorm:"column:envelope_uuid;type:uuid;not null;index:idx_event_dlq_envelope" json:"envelope_uuid"`
	EventID         string         `gorm:"column:event_id;type:varchar(128);not null;index:idx_event_dlq_event" json:"event_id"`
	FailureStage    string         `gorm:"column:failure_stage;type:varchar(32);not null" json:"failure_stage"`
	LastErrorCode   string         `gorm:"column:last_error_code;type:varchar(64)" json:"last_error_code"`
	LastErrorMsg    string         `gorm:"column:last_error_message;type:text" json:"last_error_message"`
	PayloadSnapshot datatypes.JSON `gorm:"column:payload_snapshot;type:jsonb;not null" json:"payload_snapshot"`
	Headers         datatypes.JSON `gorm:"column:headers;type:jsonb;not null;default:'{}'" json:"headers"`
	Status          string         `gorm:"column:status;type:varchar(32);not null;default:'queued';index:idx_event_dlq_status" json:"status"`
	ReplayedAt      *time.Time     `gorm:"column:replayed_at;type:timestamp with time zone" json:"replayed_at,omitempty"`
	ReplayOperator  string         `gorm:"column:replay_operator;type:varchar(128)" json:"replay_operator,omitempty"`
	TraceID         string         `gorm:"column:trace_id;type:varchar(128);index:idx_event_dlq_trace_id" json:"trace_id"`
}

func (DlqMessage) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableEventDlqMessages
}

func (m *DlqMessage) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	if m.Status == "" {
		m.Status = "queued"
	}
	return nil
}
