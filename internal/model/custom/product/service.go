package product

import "PowerX/internal/model/powermodel"

type ServiceSpecific struct {
	powermodel.PowerModel

	ParentId          int64 `gorm:"comment:上级服务Id" json:"parentId"`
	ProductId         int64 `gorm:"comment:产品ID" json:"productId"`
	IsFree            bool  `gorm:"comment:服务是否是空闲" json:"isFree"`
	Duration          int32 `gorm:"comment:服务时长" json:"duration"`
	MandatoryDuration int32 `gorm:"comment:强制服务时长" json:"mandatoryDuration"`
}

const ServiceSpecificUniqueId = powermodel.UniqueId
