package trade

import (
	"PowerX/internal/model/customerdomain"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/product"
	"PowerX/internal/types"
	"github.com/ArtisanCloud/PowerLibs/v3/object"
	"github.com/golang-module/carbon/v2"
	"time"
)

// 记录客户当次购买行为的消费信息
type Order struct {
	*powermodel.PowerModel

	Customer *customerdomain.Customer `gorm:"foreignKey:CustomerId;references:Id" json:"customerdomain"`
	Items    []*OrderItem             `gorm:"foreignKey:OrderId;references:Id" json:"items"`
	Payments []*Payment               `gorm:"foreignKey:OrderId;references:Id" json:"payments"`
	//Memberships []*membership.Membership `gorm:"foreignKey:OrderId;references:Id" json:"memberships"`
	//Reseller    *Reseller                `gorm:"foreignKey:ResellerId;references:Id" json:"reseller"`
	//CouponItems []*CouponItem            `gorm:"foreignKey:OrderId;references:Id" json:"couponItems"`

	//ResellerId     int64   `gorm:"comment:reseller_uuid" json:"resellerId"`
	CustomerId     int64       `gorm:"comment:客户Id; index" json:"customerId"`
	CartId         int64       `gorm:"comment:购物车Id; index" json:"cartId"`
	PaymentType    int8        `gorm:"comment:支付方式" json:"paymentType"`
	Type           OrderType   `gorm:"comment:订单类型" json:"type"`
	Status         OrderStatus `gorm:"comment:订单状态" json:"status"`
	OrderNumber    string      `gorm:"comment:订单号" json:"orderNumber"`
	UnitPrice      float64     `gorm:"type:decimal(10,2); comment:是实际交易价格" json:"unitPrice"`
	ListPrice      float64     `gorm:"type:decimal(10,2); comment:是订单价格" json:"listPrice"`
	Discount       float64     `gorm:"type:decimal(4,2); comment:折扣" json:"discount"`
	Comment        string      `gorm:"comment:备注" json:"comment"`
	CompletedAt    time.Time   `gorm:"comment:订单完成时间" json:"completedAt"`
	CancelledAt    time.Time   `gorm:"comment:订单取消时间" json:"cancelledAt"`
	ShippingMethod string      `gorm:"comment:物流方式" json:"shippingMethod"`
}

type OrderStatus int

const OrderUniqueId = powermodel.UniqueId
const (
	OrderStatusPending     OrderStatus = iota // 产品展示  // 待处理
	OrderStatusToBePaid                       // 待付款
	OrderStatusConfirmed                      // 已确认
	OrderStatusToBeShipped                    // 待发货
	OrderStatusShipping                       // 送货中
	OrderStatusDelivered                      // 已签收
	OrderStatusCompleted                      // 已完成
	OrderStatusCancelled                      // 已取消
	OrderStatusFailed                         // 失败
	OrderStatusRefunding                      // 退款中
	OrderStatusRefunded                       // 已退款
	OrderStatusReturned                       // 已退货
)

type OrderType int

const (
	OrderTypeNormal           OrderType = iota // 普通订单
	OrderTypePreorder                          // 预定订单
	OrderTypeCart                              // 购物车订单
	OrderTypeCustom                            // 定制订单
	OrderTypeSubscription                      // 订阅订单
	OrderTypeWholesale                         // 批发订单
	OrderTypeGift                              // 赠品订单
	OrderTypeGiftWithPurchase                  // 有赠品的订单
	OrderTypeReturn                            // 退货订单
	OrderTypeExchange                          // 换货订单
	OrderTypeReshipment                        // 补发订单
)

// 订单项，记录订单中，针对每个产品以及SKU的实际订单详情
type OrderItem struct {
	*powermodel.PowerModel

	Order            *Order                  `gorm:"foreignKey:OrderId;references:Id" json:"order"`
	ProductBookEntry *product.PriceBookEntry `gorm:"foreignKey:PriceBookEntryId;references:Id" json:"priceBook"`
	CoverImage       *types.MediaResource    `gorm:"foreignKey:CoverImageId;references:Id" json:"coverImage"`
	//Membership       *membership.Membership  `gorm:"foreignKey:OrderItemId;references:Id" json:"membership"`
	//CouponItem  *CouponItem `gorm:"foreignKey:OrderItemId;references:Id" json:"CouponItem"`

	// 正常购买信息
	OrderId          int64       `gorm:"comment:订单Id; index" json:"orderId"`
	PriceBookEntryId int64       `gorm:"comment:价格条目Id; index" json:"priceBookEntryId"`
	CustomerId       int64       `gorm:"comment:客户Id; index" json:"customerId"`
	CoverImageId     int64       `gorm:"comment:头图Id; index" json:"coverImageId"`
	Type             OrderType   `gorm:"comment:订单项类型" json:"type"`
	Status           OrderStatus `gorm:"comment:订单项状态" json:"status"`
	ProductName      string      `gorm:"comment:产品名称" json:"productName"`
	SkuNo            string      `gorm:"comment:SKU名称" json:"skuNo"`
	Quantity         int         `gorm:"comment:购买数量" json:"quantity"`
	UnitPrice        float64     `gorm:"type:decimal(10,2); comment:是单品价格" json:"unitPrice"`
	ListPrice        float64     `gorm:"type:decimal(10,2); comment:是商品标价" json:"listPrice"`
	Discount         float64     `gorm:"type:decimal(10,2); comment:折扣" json:"discount"`
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

func GenerateOrderNumber() string {
	return "SO" + carbon.Now().Format("YmdHis") + object.QuickRandom(4)
}
