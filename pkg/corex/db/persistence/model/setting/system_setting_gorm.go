// pkg/corex/db/persistence/model/setting/system_setting_gorm.go
package setting

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// 全局运营态设置（可在 UI 修改、可审计）；密钥/启动级配置仍放 ENV/文件
type SystemSetting struct {
	coremodel.PowerModel

	Key       string         `gorm:"column:key;type:varchar(128);not null;uniqueIndex:uk_sys_setting_key" json:"key"`
	ValueJSON datatypes.JSON `gorm:"column:value_json;type:jsonb"                                         json:"value_json,omitempty"`

	// 可选：分组/说明，便于后台管理界面展示
	Group       string  `gorm:"column:group;type:varchar(64);index"    json:"group,omitempty"`
	Description *string `gorm:"column:description;type:varchar(512)"   json:"description,omitempty"`
	Editable    bool    `gorm:"column:editable;default:true;index"     json:"editable"`
}

func (m *SystemSetting) TableName() string { return coremodel.PowerXSchema + "." + TableSystemSetting }
func (m *SystemSetting) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return TableSystemSetting
}
