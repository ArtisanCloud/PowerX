package product

import (
	"PowerX/internal/model/powermodel"
	product2 "PowerX/internal/model/product"
)

type ServiceSpecific struct {
	powermodel.PowerModel

	Stores   []*product2.Store  `gorm:"many2many:pivot_store_to_service;foreignKey:Id;joinForeignKey:service_id;References:Id;JoinReferences:store_id"`
	Product  *product2.Product  `gorm:"foreignKey:ProductId;references:Id" json:"product"`
	Parent   *ServiceSpecific   `gorm:"foreignKey:ParentId;references:Id" json:"parent"`
	Children []*ServiceSpecific `gorm:"foreignKey:ParentId;references:Id" json:"children"`

	ParentId          int64  `gorm:"comment:上级服务Id" json:"parentId"`
	ProductId         int64  `gorm:"comment:产品ID" json:"productId"`
	IsFree            bool   `gorm:"comment:服务是否是空闲" json:"isFree"`
	Name              string `gorm:"comment:项目名称"`
	Duration          int    `gorm:"comment:服务时长" json:"duration"`
	MandatoryDuration int    `gorm:"comment:强制服务时长" json:"mandatoryDuration"`
}

const ServiceSpecificUniqueId = powermodel.UniqueId
