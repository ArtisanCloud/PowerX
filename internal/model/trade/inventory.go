package trade

import "PowerX/internal/model/powermodel"

// 仓库
type Inventory struct {
	*powermodel.PowerModel

	WarehouseID int64 `gorm:"comment:仓库ID" json:"warehouseId"`
	ProductID   int64 `gorm:"comment:商品ID" json:"productId"`
	SkuID       int64 `gorm:"comment:SkuId" json:"skuID"`
	Quantity    int   `gorm:"comment:库存数量" json:"quantity"`
}
