package eventfabric

import (
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// EventEnvelope 记录事件发布时的完整信封信息，支撑幂等与重试。
type EventEnvelope struct {
	coremodel.PowerUUIDModel

	TenantKey      string         `gorm:"column:tenant_key;type:varchar(128);not null;index:idx_event_envelopes_tenant_key" json:"tenant_key"`
	TopicUUID      uuid.UUID      `gorm:"column:topic_uuid;type:uuid;not null;index:idx_event_envelopes_topic" json:"topic_uuid"`
	EventID        string         `gorm:"column:event_id;type:varchar(128);not null;uniqueIndex:uk_event_envelopes_event_id" json:"event_id"`
	IdempotencyKey string         `gorm:"column:idempotency_key;type:varchar(128);index:idx_event_envelopes_idempotency" json:"idempotency_key"`
	Version        string         `gorm:"column:version;type:varchar(32);not null;default:'v1'" json:"version"`
	PayloadFormat  string         `gorm:"column:payload_format;type:varchar(32);not null;default:'json'" json:"payload_format"`
	PayloadDigest  string         `gorm:"column:payload_digest;type:varchar(128);index:idx_event_envelopes_digest" json:"payload_digest"`
	Payload        datatypes.JSON `gorm:"column:payload;type:jsonb;not null" json:"payload"`
	Headers        datatypes.JSON `gorm:"column:headers;type:jsonb;not null;default:'{}'" json:"headers"`
	PublishedBy    string         `gorm:"column:published_by;type:varchar(128);not null;index:idx_event_envelopes_published_by" json:"published_by"`
	PublishedAt    time.Time      `gorm:"column:published_at;type:timestamp with time zone;not null;index:idx_event_envelopes_published_at" json:"published_at"`
	Status         string         `gorm:"column:status;type:varchar(32);not null;default:'pending';index:idx_event_envelopes_status" json:"status"`
	RetryCount     int            `gorm:"column:retry_count;type:int;not null;default:0" json:"retry_count"`
	MaxRetry       int            `gorm:"column:max_retry;type:int;not null;default:5" json:"max_retry"`
	AckTimeoutSec  int            `gorm:"column:ack_timeout_sec;type:int;not null;default:30" json:"ack_timeout_sec"`
	TraceID        string         `gorm:"column:trace_id;type:varchar(128);index:idx_event_envelopes_trace_id" json:"trace_id"`
	LastError      string         `gorm:"column:last_error;type:text" json:"last_error"`
}

func (EventEnvelope) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableEventEnvelopes
}

func (m *EventEnvelope) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	if m.PublishedAt.IsZero() {
		m.PublishedAt = time.Now().UTC()
	}
	if m.Status == "" {
		m.Status = "pending"
	}
	return nil
}
