package reservationcenter

import (
	"PowerX/internal/model/powermodel"
)

type ScheduleConfig struct {
	powermodel.PowerModel

	StoreId     int64  `gorm:"comment:店铺Id" json:"storeId"`
	Capacity    int32  `gorm:"comment:最大客服服务容量" json:"capacity"`
	Name        string `gorm:"comment:名字" json:"name"`
	Description string `gorm:"comment:描述" json:"description"`
	IsActive    string `gorm:"comment:开放状态" json:"isActive"`
}

const ScheduleConfigUniqueId = powermodel.UniqueId
