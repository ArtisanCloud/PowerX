package product

import (
	"PowerX/internal/model/powermodel"
)

// SKU 数据表结构
type SKU struct {
	powermodel.PowerModel

	PivotSKUToSpecificOptions []*PivotSkuToSpecificOption `gorm:"foreignKey:SKUId;references:Id" json:"pivotSKUToSpecificOptions"`

	ProductId int64  `gorm:"index:idx_product_id;not null;" json:"productId"`
	SkuNo     string `gorm:"comment:SKU编号" json:"sku"`
	Inventory int    `gorm:"comment:库存数量" json:"inventory"`
}

const TableNameSKU = "sku"
