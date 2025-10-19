package eventfabric

import (
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AclBinding 记录租户下主体在特定主题上的操作权限。
type AclBinding struct {
	coremodel.PowerUUIDModel

	TenantID      uint64    `gorm:"column:tenant_id;type:bigint;not null;index:idx_event_acl_tenant" json:"tenant_id"`
	TenantKey     string    `gorm:"column:tenant_key;type:varchar(128);not null;index:idx_event_acl_tenant_key;uniqueIndex:uk_event_acl_binding,priority:1" json:"tenant_key"`
	TopicUUID     uuid.UUID `gorm:"column:topic_uuid;type:uuid;not null;index:idx_event_acl_topic;uniqueIndex:uk_event_acl_binding,priority:2" json:"topic_uuid"`
	PrincipalType string    `gorm:"column:principal_type;type:varchar(32);not null" json:"principal_type"`
	PrincipalID   string    `gorm:"column:principal_id;type:varchar(128);not null;uniqueIndex:uk_event_acl_binding,priority:3" json:"principal_id"`
	Action        string    `gorm:"column:action;type:varchar(32);not null;uniqueIndex:uk_event_acl_binding,priority:4" json:"action"`

	ExpiresAt     *time.Time `gorm:"column:expires_at" json:"expires_at,omitempty"`
	GrantedBy     string     `gorm:"column:granted_by;type:varchar(128)" json:"granted_by,omitempty"`
	Justification string     `gorm:"column:justification;type:text" json:"justification,omitempty"`
	AuditRef      string     `gorm:"column:audit_ref;type:varchar(128)" json:"audit_ref,omitempty"`
	Status        int16      `gorm:"column:status;default:1;index" json:"status"`
}

func (m *AclBinding) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableEventAclBindings
}

func (m *AclBinding) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	return nil
}

func (m *AclBinding) UniqueColumns() []string {
	return []string{"tenant_key", "topic_uuid", "principal_id", "action"}
}
