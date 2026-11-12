package notification

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// CreationReceipt 保存独立于通知删除/已读状态的创建凭据，不参与软删除。
type CreationReceipt struct {
	UUID             uuid.UUID      `gorm:"column:uuid;type:uuid;primaryKey"`
	TenantUUID       string         `gorm:"column:tenant_uuid;type:uuid;not null;uniqueIndex:ux_notification_creation_scope"`
	ServiceOwner     string         `gorm:"column:service_owner;type:varchar(512);not null;uniqueIndex:ux_notification_creation_scope"`
	KeyHash          string         `gorm:"column:key_hash;type:char(64);not null;uniqueIndex:ux_notification_creation_scope"`
	RequestHash      string         `gorm:"column:request_hash;type:char(64);not null"`
	NotificationUUID uuid.UUID      `gorm:"column:notification_uuid;type:uuid;not null"`
	ResponseJSON     datatypes.JSON `gorm:"column:response_json;type:jsonb;not null"`
}

func (CreationReceipt) TableName() string {
	return coremodel.PowerXSchema + ".system_notification_creation_receipts"
}
