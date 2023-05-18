package product

import (
	"PowerX/internal/model/powermodel"
	"gorm.io/datatypes"
)

// SKU 数据表结构
type SKU struct {
	powermodel.PowerModel

	PivotSkuToSpecificOptions []*PivotSkuToSpecificOption `gorm:"foreignKey:SkuId;references:Id" json:"pivotSkuToSpecificOptions"`
	PriceBookEntry            *PriceBookEntry             `gorm:"foreignKey:SkuId;references:Id" json:"priceBookEntry"`

	ProductId int64          `gorm:"index:idx_product_id;not null;" json:"productId"`
	SkuNo     string         `gorm:"comment:SKU编号" json:"sku"`
	Inventory int            `gorm:"comment:库存数量" json:"inventory"`
	OptionIds datatypes.JSON `gorm:"comment:规格Ids" json:"OptionIds"`
}

const TableNameSKU = "sku"
