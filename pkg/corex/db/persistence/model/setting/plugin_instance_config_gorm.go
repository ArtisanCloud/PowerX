// pkg/corex/db/persistence/model/setting/plugin_instance_config.go
package setting

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// 插件“租户态”的启停与参数；版本/安装真源仍由 JSONRegistry 管
type PluginInstanceConfig struct {
	coremodel.PowerModel

	TenantID uint64 `gorm:"column:tenant_id;not null;index:idx_plugincfg_tpk,priority:1" json:"tenant_id"`
	PluginID string `gorm:"column:plugin_id;type:varchar(128);not null;index:idx_plugincfg_tpk,priority:2" json:"plugin_id"`

	// 可按 key 细分，也可以使用固定 schema（灵活度更高：KV）
	Key       string         `gorm:"column:key;type:varchar(128);not null;index:idx_plugincfg_tpk,priority:3;uniqueIndex:uk_plugincfg_tpk,priority:1" json:"key"`
	ValueJSON datatypes.JSON `gorm:"column:value_json;type:jsonb"                                                                                      json:"value_json,omitempty"`

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
