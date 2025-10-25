package eventfabric

import (
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ReplayRequest 表示一次事件回放任务。
type ReplayRequest struct {
	coremodel.PowerUUIDModel

	TenantID    uint64    `gorm:"column:tenant_id;type:bigint;not null;index:idx_event_replay_tenant" json:"tenant_id"`
	TenantKey   string    `gorm:"column:tenant_key;type:varchar(128);not null;index:idx_event_replay_tenant_key" json:"tenant_key"`
	TopicUUID   uuid.UUID `gorm:"column:topic_uuid;type:uuid;not null;index:idx_event_replay_topic" json:"topic_uuid"`
	TraceID     string    `gorm:"column:trace_id;type:varchar(128);index:idx_event_replay_trace" json:"trace_id"`
	Shadow      bool      `gorm:"column:shadow;not null;default:false" json:"shadow"`
	VersionMode string    `gorm:"column:version_mode;type:varchar(32)" json:"version_mode"`

	TimeRangeStart     time.Time `gorm:"column:time_range_start;type:timestamp with time zone" json:"time_range_start"`
	TimeRangeEnd       time.Time `gorm:"column:time_range_end;type:timestamp with time zone" json:"time_range_end"`
	FilterSubscriberID string    `gorm:"column:filter_subscriber_id;type:varchar(128)" json:"filter_subscriber_id"`

	Status        string `gorm:"column:status;type:varchar(32);not null;default:'pending';index:idx_event_replay_status" json:"status"`
	IssuedBy      string `gorm:"column:issued_by;type:varchar(128)" json:"issued_by"`
	Reason        string `gorm:"column:reason;type:text" json:"reason"`
	ResultCount   int    `gorm:"column:result_count;type:int" json:"result_count"`
	FailureReason string `gorm:"column:failure_reason;type:text" json:"failure_reason"`

	SubmittedAt time.Time  `gorm:"column:submitted_at;type:timestamp with time zone" json:"submitted_at"`
	CompletedAt *time.Time `gorm:"column:completed_at;type:timestamp with time zone" json:"completed_at"`
	CancelledAt *time.Time `gorm:"column:cancelled_at;type:timestamp with time zone" json:"cancelled_at"`
}

const (
	ReplayStatusPending   = "pending"
	ReplayStatusRunning   = "running"
	ReplayStatusCompleted = "completed"
	ReplayStatusFailed    = "failed"
	ReplayStatusCancelled = "cancelled"
)

func (ReplayRequest) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableEventReplayRequests
}

func (r *ReplayRequest) BeforeCreate(tx *gorm.DB) error {
	if r.UUID == uuid.Nil {
		r.UUID = uuid.New()
	}
	if r.SubmittedAt.IsZero() {
		r.SubmittedAt = time.Now().UTC()
	}
	if r.Status == "" {
		r.Status = ReplayStatusPending
	}
	return nil
}
