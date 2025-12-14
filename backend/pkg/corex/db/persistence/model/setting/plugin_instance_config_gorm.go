// pkg/corex/db/persistence/model/setting/plugin_instance_config.go
package setting

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// 插件“租户态”的启停与参数；版本/安装真源仍由 JSONRegistry 管
type PluginInstanceConfig struct {
	coremodel.PowerModel

	TenantUUID string `gorm:"column:tenant_uuid;type:varchar(128);not null;uniqueIndex:uk_plugincfg_tpk,priority:1" json:"tenant_uuid"`
	PluginID   string `gorm:"column:plugin_id;type:varchar(128);not null;uniqueIndex:uk_plugincfg_tpk,priority:2" json:"plugin_id"`
	Key        string `gorm:"column:key;type:varchar(128);not null;uniqueIndex:uk_plugincfg_tpk,priority:3" json:"key"`

	ValueJSON datatypes.JSON `gorm:"column:value_json;type:jsonb" json:"value_json,omitempty"`

	Enabled bool `gorm:"column:enabled;default:true;index" json:"enabled"`
}

func (m *PluginInstanceConfig) TableName() string {
	return coremodel.PowerXSchema + "." + TablePluginInstanceConfig
}
func (m *PluginInstanceConfig) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return TablePluginInstanceConfig
}
