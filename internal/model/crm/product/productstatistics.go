package product

import "PowerX/internal/model/powermodel"

type ProductStatistics struct {
	powermodel.PowerModel

	ProductId             int64 `gorm:"comment:产品Id" json:"productId"`
	SoldAmount            int64 `gorm:"comment:销量" json:"soldAmount"`
	InventoryQuantity     int64 `gorm:"comment:库存;" json:"inventoryQuantity"`
	ViewCount             int64 `gorm:"comment:浏览量;" json:"viewCount"`
	BaseSoldAmount        int64 `gorm:"comment:销量" json:"baseSoldAmount"`
	BaseInventoryQuantity int64 `gorm:"comment:库存;" json:"baseInventoryQuantity"`
	BaseViewCount         int64 `gorm:"comment:浏览量;" json:"baseViewCount"`
}

const ProductStatisticsUniqueId = powermodel.UniqueId
