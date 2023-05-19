package trade

import (
	"PowerX/internal/model/customerdomain"
	"PowerX/internal/model/membership"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/product"
	"time"
)

// 记录客户当次购买行为的消费信息
type Order struct {
	*powermodel.PowerModel

	Customer    *customerdomain.Customer `gorm:"foreignKey:AccountId;references:Id" json:"customer"`
	OrderItems  []*OrderItem             `gorm:"foreignKey:OrderId;references:Id" json:"orderItems"`
	Payments    []*Payment               `gorm:"foreignKey:OrderId;references:Id" json:"payments"`
	Memberships []*membership.Membership `gorm:"foreignKey:OrderId;references:Id" json:"memberships"`
	//Reseller    *Reseller                `gorm:"foreignKey:ResellerId;references:Id" json:"reseller"`
	//CouponItems []*CouponItem            `gorm:"foreignKey:OrderId;references:Id" json:"couponItems"`

	//ResellerId     int64   `gorm:"comment:reseller_uuid" json:"resellerId"`
	CustomerId     int64       `gorm:"comment:客户Id; index" json:"customerId"`
	CartId         int64       `gorm:"comment:购物车Id; index" json:"cartId"`
	PaymentType    int8        `gorm:"comment:支付方式" json:"paymentType"`
	Type           OrderType   `gorm:"comment:订单类型" json:"type"`
	Status         OrderStatus `gorm:"comment:订单状态" json:"status"`
	OrderNumber    string      `gorm:"comment:订单号" json:"orderNumber"`
	Discount       float64     `gorm:"type:decimal(4,2); comment:折扣" json:"discount"`
	ListPrice      float64     `gorm:"type:decimal(10,2); comment:是订单价格" json:"listPrice"`
	SellingPrice   float64     `gorm:"type:decimal(10,2); comment:是实际交易价格" json:"sellingPrice"`
	Comment        string      `gorm:"comment:备注" json:"comment"`
	CompletedAt    time.Time   `gorm:"comment:订单完成时间" json:"completedAt"`
	CancelledAt    time.Time   `gorm:"comment:订单取消时间" json:"cancelledAt"`
	ShippingMethod string      `gorm:"comment:物流方式" json:"shippingMethod"`
}

type OrderStatus int

const (
	OrderStatusPending    OrderStatus = 0 // 待处理
	OrderStatusConfirmed  OrderStatus = 1 // 已确认
	OrderStatusInProgress OrderStatus = 2 // 进行中
	OrderStatusCompleted  OrderStatus = 3 // 已完成
	OrderStatusCancelled  OrderStatus = 4 // 已取消
	OrderStatusFailed     OrderStatus = 5 // 失败
	OrderStatusRefunded   OrderStatus = 6 // 已退款
	OrderStatusReturned   OrderStatus = 7 // 已退货
)

type OrderType int

const (
	OrderTypeNormal           OrderType = 0 // 普通订单
	OrderTypePreorder         OrderType = 1 // 预定订单
	OrderTypeCustom           OrderType = 2 // 定制订单
	OrderTypeSubscription     OrderType = 3 // 订阅订单
	OrderTypeWholesale        OrderType = 4 // 批发订单
	OrderTypeGift             OrderType = 5 // 赠品订单
	OrderTypeGiftWithPurchase OrderType = 6 // 有赠品的订单
	OrderTypeReturn           OrderType = 7 // 退货订单
	OrderTypeExchange         OrderType = 8 // 换货订单
	OrderTypeReshipment       OrderType = 9 // 补发订单
)

// 订单项，记录订单中，针对每个产品以及SKU的实际订单详情
type OrderItem struct {
	*powermodel.PowerModel

	Order       *Order                 `gorm:"foreignKey:OrderId;references:Id" json:"order"`
	Product     *product.Product       `gorm:"foreignKey:ProductId;references:Id" json:"product"`
	ProductBook *product.PriceBook     `gorm:"foreignKey:PriceBookEntryId;references:Id" json:"priceBook"`
	Membership  *membership.Membership `gorm:"foreignKey:OrderItemId;references:Id" json:"membership"`
	//CouponItem  *CouponItem `gorm:"foreignKey:OrderItemId;references:Id" json:"CouponItem"`

	// 正常购买信息
	OrderId          int64       `gorm:"comment:订单Id; index" json:"orderId"`
	PriceBookEntryId int64       `gorm:"comment:价格条目Id; index" json:"priceBookEntryId"`
	CustomerId       int64       `gorm:"comment:客户Id; index" json:"customerId"`
	Type             OrderType   `gorm:"comment:订单项类型" json:"type"`
	Status           OrderStatus `gorm:"comment:订单项状态" json:"status"`
	Quantity         int8        `gorm:"comment:购买数量" json:"quantity"`
	UnitPrice        float64     `gorm:"type:decimal(10,2); comment:是单品价格" json:"unitPrice"`
	ListPrice        float64     `gorm:"type:decimal(10,2); comment:是商品标价" json:"listPrice"`
	SellingPrice     float64     `gorm:"type:decimal(10,2); comment:是实际交易价格" json:"sellingPrice"`
}

type OrderStatusTransition struct {
	*powermodel.PowerModel

	OrderId        int64       `gorm:"comment:订单Id; index" json:"orderId"`
	FromStatus     OrderStatus `gorm:"comment:原状态" json:"fromStatus"`
	ToStatus       OrderStatus `gorm:"comment:目标状态" json:"toStatus"`
	Remark         string      `gorm:"comment:备注" json:"remark"`
	CreatorId      int64       `gorm:"comment:创建者Id" json:"creatorId"`
	CreatorName    string      `gorm:"comment:创建者名字" json:"creatorName"`
	TransitionTime time.Time   `gorm:"comment:状态转换时间" json:"transitionTime"`
}
