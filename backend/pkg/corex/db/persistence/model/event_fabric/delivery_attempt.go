package eventfabric

import (
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DeliveryAttempt 记录单个订阅者的投递状态与重试信息。
type DeliveryAttempt struct {
	coremodel.PowerUUIDModel

	TenantID      uint64     `gorm:"column:tenant_id;type:bigint;not null;index:idx_event_delivery_attempts_tenant" json:"tenant_id"`
	TenantKey     string     `gorm:"column:tenant_key;type:varchar(128);not null;index:idx_event_delivery_attempts_tenant_key;uniqueIndex:uk_event_delivery_attempt,priority:1" json:"tenant_key"`
	EnvelopeUUID  uuid.UUID  `gorm:"column:envelope_uuid;type:uuid;not null;index:idx_event_delivery_attempts_envelope" json:"envelope_uuid"`
	EventID       string     `gorm:"column:event_id;type:varchar(128);not null;index:idx_event_delivery_attempts_event;uniqueIndex:uk_event_delivery_attempt,priority:2" json:"event_id"`
	SubscriberID  string     `gorm:"column:subscriber_id;type:varchar(128);not null;index:idx_event_delivery_attempts_subscriber;uniqueIndex:uk_event_delivery_attempt,priority:3" json:"subscriber_id"`
	DeliveryNo    int        `gorm:"column:delivery_no;type:int;not null;default:1;uniqueIndex:uk_event_delivery_attempt,priority:4" json:"delivery_no"`
	Status        string     `gorm:"column:status;type:varchar(32);not null;default:'pending';index:idx_event_delivery_attempts_status" json:"status"`
	LatencyMs     int        `gorm:"column:latency_ms;type:int;not null;default:0" json:"latency_ms"`
	LastErrorCode string     `gorm:"column:last_error_code;type:varchar(64)" json:"last_error_code"`
	NackReason    string     `gorm:"column:nack_reason;type:text" json:"nack_reason"`
	ScheduledAt   *time.Time `gorm:"column:scheduled_at;type:timestamp with time zone;index:idx_event_delivery_attempts_schedule" json:"scheduled_at,omitempty"`
	LastAttemptAt *time.Time `gorm:"column:last_attempt_at;type:timestamp with time zone" json:"last_attempt_at,omitempty"`
	AckedAt       *time.Time `gorm:"column:acked_at;type:timestamp with time zone" json:"acked_at,omitempty"`
	TraceID       string     `gorm:"column:trace_id;type:varchar(128);index:idx_event_delivery_attempts_trace_id" json:"trace_id"`
}

func (DeliveryAttempt) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableEventDeliveryAttempts
}

func (m *DeliveryAttempt) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	if m.DeliveryNo == 0 {
		m.DeliveryNo = 1
	}
	if m.Status == "" {
		m.Status = "pending"
	}
	return nil
}
