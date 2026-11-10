package customer

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// ClientApplication 属于一个租户；应用形态与身份提供方连接分别建模。
type ClientApplication struct {
	coremodel.PowerUUIDModel
	TenantUUID string         `gorm:"column:tenant_uuid;type:uuid;not null;index" json:"tenant_uuid"`
	Name       string         `gorm:"size:128;not null" json:"name"`
	Kind       string         `gorm:"size:32;not null;index" json:"kind"`
	Status     string         `gorm:"size:32;not null;default:disabled" json:"status"`
	Version    int64          `gorm:"not null;default:1" json:"version"`
	PluginIDs  datatypes.JSON `gorm:"column:plugin_ids;type:jsonb;not null" json:"-"`
}

func (ClientApplication) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableCustomerClientApplications
}

// ClientAppBinding 可为一个应用关联多个不同 Provider；密文永不进入公开 DTO。
type ClientAppBinding struct {
	coremodel.PowerUUIDModel
	TenantUUID      string            `gorm:"column:tenant_uuid;type:uuid;not null;uniqueIndex:uk_customer_client_binding,priority:1;index" json:"-"`
	ApplicationUUID string            `gorm:"column:application_uuid;type:uuid;not null;index" json:"application_uuid"`
	Provider        string            `gorm:"size:32;not null" json:"provider"`
	Platform        string            `gorm:"size:64;not null;uniqueIndex:uk_customer_client_binding,priority:2" json:"platform"`
	AppID           string            `gorm:"column:app_id;size:128;not null;uniqueIndex:uk_customer_client_binding,priority:3" json:"app_id"`
	SealedSecret    datatypes.JSONMap `gorm:"column:sealed_secret;type:jsonb" json:"-"`
}

func (ClientAppBinding) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableCustomerClientAppBindings
}
