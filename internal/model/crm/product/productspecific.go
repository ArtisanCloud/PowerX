package product

import (
	"PowerX/internal/model"
	"PowerX/internal/model/powermodel"
)

type ProductSpecific struct {
	Options []*SpecificOption `gorm:"foreignKey:ProductSpecificId" json:"options"`

	powermodel.PowerModel

	ProductId int64  `gorm:"comment:产品Id; index;not null" json:"productId"`
	Name      string `gorm:"comment:规格名称;" json:"name"`
}

func (mdl *ProductSpecific) TableName() string {
	return model.PowerXSchema + "." + model.TableNameProductSpecific
}

func (mdl *ProductSpecific) GetTableName(needFull bool) string {
	tableName := model.TableNameProductSpecific
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}

type SpecificOption struct {
	powermodel.PowerModel

	ProductSpecificId int64  `gorm:"comment: 产品规格Id; index;not null" json:"productSpecificId"`
	Name              string `gorm:"comment: 规格项名称; not null" json:"name"`
	IsActivated       bool   `gorm:"comment: 是否被激活;" json:"isActivated"`
}

const ProductSpecificUniqueId = powermodel.UniqueId
const SpecificOptionUniqueId = powermodel.UniqueId

func (mdl *SpecificOption) TableName() string {
	return model.PowerXSchema + "." + model.TableNameSpecificOption
}

func (mdl *SpecificOption) GetTableName(needFull bool) string {
	tableName := model.TableNameSpecificOption
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}
