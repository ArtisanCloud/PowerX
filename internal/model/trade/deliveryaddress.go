package trade

import "PowerX/internal/model/powermodel"

// 订单发货地址
type DeliveryAddress struct {
	*powermodel.PowerModel

	OrderId int64 `gorm:"comment:订单Id; index" json:"orderId"`
	ShippingAddress
}
