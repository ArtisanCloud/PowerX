package eventfabric

import (
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
)

// SubscriptionOffset 记录订阅者最近确认的投递位置。
type SubscriptionOffset struct {
	TenantKey    string    `gorm:"column:tenant_key;type:varchar(128);not null;primaryKey" json:"tenant_key"`
	TopicUUID    uuid.UUID `gorm:"column:topic_uuid;type:uuid;not null;primaryKey" json:"topic_uuid"`
	SubscriberID string    `gorm:"column:subscriber_id;type:varchar(128);not null;primaryKey" json:"subscriber_id"`

	LastEventUUID  uuid.UUID  `gorm:"column:last_event_uuid;type:uuid" json:"last_event_uuid"`
	LastEventID    string     `gorm:"column:last_event_id;type:varchar(128)" json:"last_event_id"`
	LastAckAt      *time.Time `gorm:"column:last_ack_at;type:timestamp with time zone" json:"last_ack_at,omitempty"`
	DeliveryCursor string     `gorm:"column:delivery_cursor;type:varchar(256)" json:"delivery_cursor,omitempty"`
	DeliveryMode   string     `gorm:"column:delivery_mode;type:varchar(32);not null;default:'stream'" json:"delivery_mode"`
	TraceID        string     `gorm:"column:trace_id;type:varchar(128);index:idx_event_subscription_offsets_trace_id" json:"trace_id"`

	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (SubscriptionOffset) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableEventSubscriptionOffsets
}
