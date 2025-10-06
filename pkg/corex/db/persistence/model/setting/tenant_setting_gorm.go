// pkg/corex/db/persistence/model/setting/tenant_setting_gorm.go
package setting

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// 租户覆盖层：同一 key 在租户内唯一；读取时合并 tenant > system > 文件/ENV > 默认
type TenantSetting struct {
	coremodel.PowerModel

	TenantID  uint64         `gorm:"column:tenant_id;not null;index:idx_tenant_setting_tk,priority:1" json:"tenant_id"`
	Key       string         `gorm:"column:key;type:varchar(128);not null;index:idx_tenant_setting_tk,priority:2;uniqueIndex:uk_tenant_setting_tk,priority:1" json:"key"`
	ValueJSON datatypes.JSON `gorm:"column:value_json;type:jsonb"                                     json:"value_json,omitempty"`

	Group       string  `gorm:"column:group;type:varchar(64);index"  json:"group,omitempty"`
	Description *string `gorm:"column:description;type:varchar(512)" json:"description,omitempty"`
	Editable    bool    `gorm:"column:editable;default:true;index"   json:"editable"`
}

func (m *TenantSetting) TableName() string { return coremodel.PowerXSchema + "." + TableTenantSetting }
func (m *TenantSetting) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return TableTenantSetting
}
