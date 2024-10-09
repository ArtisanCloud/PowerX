package trade

import (
	"PowerX/internal/model"
	"PowerX/internal/model/powermodel"
)

// 仓库
type Warehouse struct {
	*powermodel.PowerModel

	Name          string `gorm:"comment:仓库名称" json:"name"`
	Address       string `gorm:"comment:仓库地址" json:"address"`
	City          string `gorm:"comment:城市" json:"city"`
	Region        string `gorm:"comment:区域" json:"region"`
	Type          string `gorm:"comment:仓库类型" json:"type"`
	Capacity      int    `gorm:"comment:仓库容量" json:"capacity"`
	ContactPerson string `gorm:"comment:联系人" json:"contactPerson"`
	ContactPhone  string `gorm:"comment:联系电话" json:"contactPhone"`
	IsActive      bool   `gorm:"comment:是否活动" json:"isActive"`
}

func (mdl *Warehouse) TableName() string {
	return model.PowerXSchema + "." + model.TableNameWarehouse
}

func (mdl *Warehouse) GetTableName(needFull bool) string {
	tableName := model.TableNameWarehouse
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}
