package notification

import (
	"time"

	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// Notification 记录系统通知（按租户/成员隔离）。
type Notification struct {
	coremodel.PowerUUIDModel

	TenantUUID  string         `gorm:"column:tenant_uuid;type:uuid;not null;index" json:"tenant_uuid"`
	MemberUUID  string         `gorm:"column:member_uuid;type:varchar(64);index" json:"member_uuid,omitempty"`
	Title       string         `gorm:"column:title;type:varchar(255);not null" json:"title"`
	Content     string         `gorm:"column:content;type:text" json:"content"`
	Type        string         `gorm:"column:type;type:varchar(32);not null;index" json:"type"`
	Category    string         `gorm:"column:category;type:varchar(64);not null;index" json:"category"`
	IsRead      bool           `gorm:"column:is_read;type:boolean;not null;default:false;index" json:"is_read"`
	IsImportant bool           `gorm:"column:is_important;type:boolean;not null;default:false;index" json:"is_important"`
	ReadAt      *time.Time     `gorm:"column:read_at" json:"read_at,omitempty"`
	RelatedID   string         `gorm:"column:related_id;type:varchar(128)" json:"related_id,omitempty"`
	RelatedType string         `gorm:"column:related_type;type:varchar(64)" json:"related_type,omitempty"`
	Actions     datatypes.JSON `gorm:"column:actions;type:jsonb;default:'[]'" json:"actions,omitempty"`
	Metadata    datatypes.JSON `gorm:"column:metadata;type:jsonb;default:'{}'" json:"metadata,omitempty"`
}

func (Notification) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableSystemNotifications
}
