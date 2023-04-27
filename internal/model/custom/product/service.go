package product

import (
	"PowerX/internal/model/powermodel"
	product2 "PowerX/internal/model/product"
)

type ServiceSpecific struct {
	powermodel.PowerModel

	Product *product2.Product `gorm:"foreignKey:ProductId;references:Id" json:"product"`

	ParentId          int64 `gorm:"comment:上级服务Id" json:"parentId"`
	ProductId         int64 `gorm:"comment:产品ID" json:"productId"`
	IsFree            bool  `gorm:"comment:服务是否是空闲" json:"isFree"`
	Duration          int32 `gorm:"comment:服务时长" json:"duration"`
	MandatoryDuration int32 `gorm:"comment:强制服务时长" json:"mandatoryDuration"`
}

const ServiceSpecificUniqueId = powermodel.UniqueId
