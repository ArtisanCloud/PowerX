package eventfabric

import (
	"fmt"
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// TopicDefinition 描述事件主题的治理信息。
type TopicDefinition struct {
	coremodel.PowerUUIDModel

	TenantKey       string         `gorm:"column:tenant_key;type:varchar(128);not null;index:idx_event_topics_tenant_key;index:idx_event_topics_composite,priority:1" json:"tenant_key"`
	Namespace       string         `gorm:"column:namespace;type:varchar(128);not null;index:idx_event_topics_namespace;index:idx_event_topics_composite,priority:3" json:"namespace"`
	Name            string         `gorm:"column:name;type:varchar(128);not null;index:idx_event_topics_name;index:idx_event_topics_composite,priority:4" json:"name"`
	FullTopic       string         `gorm:"column:full_topic;type:varchar(256);not null;uniqueIndex:uk_event_topic_full_topic" json:"full_topic"`
	Lifecycle       TopicLifecycle `gorm:"column:lifecycle_status;type:varchar(32);not null;index:idx_event_topics_lifecycle" json:"lifecycle"`
	PayloadFormat   string         `gorm:"column:payload_format;type:varchar(32);not null;default:json" json:"payload_format"`
	RetentionPolicy datatypes.JSON `gorm:"column:retention_policy;type:jsonb;default:'{}'" json:"retention_policy"`
	VersioningMode  string         `gorm:"column:versioning_mode;type:varchar(32);not null;default:'strict'" json:"versioning_mode"`
	MaxRetry        int            `gorm:"column:max_retry;not null;default:5" json:"max_retry"`
	AckTimeoutSec   int            `gorm:"column:ack_timeout_sec;not null;default:30" json:"ack_timeout_sec"`
	Metadata        datatypes.JSON `gorm:"column:metadata;type:jsonb;default:'{}'" json:"metadata"`
	CreatedBy       string         `gorm:"column:created_by;type:varchar(128)" json:"created_by"`
	DeprecatedAt    *time.Time     `gorm:"column:deprecated_at" json:"deprecated_at,omitempty"`
	TraceID         string         `gorm:"column:trace_id;type:varchar(128);index:idx_event_topics_trace_id" json:"trace_id,omitempty"`
	Status          int16          `gorm:"column:status;default:1;index:idx_event_topics_status" json:"status"`
}

func (m *TopicDefinition) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableEventTopics
}

func (m *TopicDefinition) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	return m.ensureDerivedFields()
}

func (m *TopicDefinition) BeforeUpdate(tx *gorm.DB) error {
	return m.ensureDerivedFields()
}

func (m *TopicDefinition) ensureDerivedFields() error {
	ns := strings.TrimSpace(m.Namespace)
	name := strings.TrimSpace(m.Name)
	tenant := strings.TrimSpace(m.TenantKey)

	if ns == "" || name == "" {
		return fmt.Errorf("namespace and name cannot be empty")
	}
	if tenant == "" {
		tenant = "global"
		m.TenantKey = tenant
	}
	m.FullTopic = fmt.Sprintf("%s.%s.%s", tenant, ns, name)
	return nil
}
