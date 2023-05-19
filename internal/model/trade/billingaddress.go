package trade

import "PowerX/internal/model/powermodel"

// 订单开票地址
type BillingAddress struct {
	*powermodel.PowerModel

	OrderId int64 `gorm:"comment:订单Id; index" json:"orderId"`
	ShippingAddress
}
