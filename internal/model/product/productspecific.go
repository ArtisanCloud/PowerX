package product

import "PowerX/internal/model/powermodel"

type ProductSpecific struct {
	Options []*SpecificOption `gorm:"foreignKey:ProductSpecificId" json:"options"`

	powermodel.PowerModel

	ProductId int64  `gorm:"comment:产品Id; index;not null" json:"productId"`
	Name      string `gorm:"comment:规格名称;" json:"name"`
}

type SpecificOption struct {
	powermodel.PowerModel

	ProductSpecificId int64  `gorm:"comment: 产品规格Id; index;not null" json:"productSpecificId"`
	Name              string `gorm:"comment: 规格项名称; not null" json:"name"`
	IsActivated       bool   `gorm:"comment:是否被激活; column:is_activated;" json:"isActivated,optional"`
}

const ProductSpecificUniqueId = "name"
