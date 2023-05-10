package trade

import (
	"PowerX/internal/model/customerdomain"
	"PowerX/internal/model/membership"
	"PowerX/internal/model/powermodel"
)

type Order struct {
	*powermodel.PowerModel

	Customer    *customerdomain.Customer `gorm:"foreignKey:AccountId;references:Id" json:"customer"`
	OrderItems  []*OrderItem             `gorm:"foreignKey:OrderId;references:Id" json:"orderItems"`
	Payments    []*Payment               `gorm:"foreignKey:OrderId;references:Id" json:"payments"`
	Memberships []*membership.Membership `gorm:"foreignKey:OrderId;references:Id" json:"memberships"`
	//Reseller    *Reseller                `gorm:"foreignKey:ResellerId;references:Id" json:"reseller"`
	//CouponItems []*CouponItem            `gorm:"foreignKey:OrderId;references:Id" json:"couponItems"`

	//ResellerId     int64   `gorm:"comment:reseller_uuid" json:"resellerId"`
	CustomerId   int64   `gorm:"comment:客户Id" json:"customerId"`
	PaymentType  int8    `gorm:"comment:支付方式" json:"paymentType"`
	Type         int8    `gorm:"comment:type" json:"type"`
	Status       int8    `gorm:"comment:status" json:"status"`
	OrderNumber  string  `gorm:"comment:订单号" json:"orderNumber"`
	Discount     float64 `gorm:"type:decimal(4,2); comment:折扣" json:"discount"`
	ListPrice    float64 `gorm:"type:decimal(10,2); comment:是订单价格" json:"listPrice"`
	SellingPrice float64 `gorm:"type:decimal(10,2); comment:是实际交易价格" json:"sellingPrice"`
	Comment      string  `gorm:"comment:备注" json:"comment"`
}
