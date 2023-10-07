package product

import "PowerX/internal/model/powermodel"

type ProductStatistics struct {
	powermodel.PowerModel

	ProductId         int64 `gorm:"comment:产品Id" json:"productId"`
	SoldAmount        int64 `gorm:"comment:销量" json:"soldAmount"`
	InventoryQuantity int64 `gorm:"comment:库存;" json:"inventoryQuantity"`
	ViewCount         int64 `gorm:"comment:浏览量;" json:"viewCount"`
}

const ProductStatisticsUniqueId = powermodel.UniqueId
