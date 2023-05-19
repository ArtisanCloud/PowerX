package trade

import (
	"PowerX/internal/model/powermodel"
)

type Cart struct {
	*powermodel.PowerModel
	Items []*CartItem `gorm:"foreignKey:CartId" json:"items"`

	CustomerId int64      `gorm:"comment:客户Id" json:"customerId"`
	Status     CartStatus `gorm:"comment:购物车状态" json:"status"`
}

type CartItem struct {
	*powermodel.PowerModel

	CartId         int64   `gorm:"comment:购物车Id; index" json:"cartId"`
	ProductId      int64   `gorm:"comment:商品Id; index" json:"productId"`
	ProductName    string  `gorm:"comment:商品名称" json:"productName"`
	ProductPrice   float64 `gorm:"comment:商品价格" json:"productPrice"`
	Quantity       int     `gorm:"comment:商品数量" json:"quantity"`
	Specifications string  `gorm:"comment:商品规格" json:"specifications"`
	ImageURL       string  `gorm:"comment:商品图片URL" json:"imageUrl"`
}

type CartStatus string

const (
	CartStatusActive   CartStatus = "active"
	CartStatusInactive CartStatus = "inactive"
	CartStatusPending  CartStatus = "pending"
	CartStatusPaid     CartStatus = "paid"
	CartStatusCanceled CartStatus = "canceled"
)
