package trade

import (
	"PowerX/internal/model/customerdomain"
	"PowerX/internal/model/powermodel"
	"time"
)

// 记录客户当次退款行为
type RefundOrder struct {
	*powermodel.PowerModel

	Customer         *customerdomain.Customer `gorm:"foreignKey:CustomerId;references:Id" json:"customerdomain"`
	RefundOrderItems []*RefundOrderItem       `gorm:"foreignKey:RefundOrderId;references:Id" json:"refundOrderItems"`

	//ResellerId     int64   `gorm:"comment:reseller_uuid" json:"resellerId"`
	CustomerId     int64        `gorm:"comment:客户Id; index" json:"customerId"`
	OrderId        int64        `gorm:"comment:订单号Id; index" json:"orderId"`
	RefundNumber   string       `gorm:"comment:退款订单号; index" json:"refundNumber"`
	RefundStatus   RefundStatus `gorm:"comment:退款状态" json:"refundStatus"`
	RefundAmount   float64      `gorm:"type:decimal(10,2); comment:退款金额" json:"refundAmount"`
	RefundReason   string       `gorm:"comment:退款原因" json:"refundReason"`
	RefundApproved bool         `gorm:"comment:退款是否已批准" json:"refundApproved"`
	RefundDate     time.Time    `gorm:"comment:退款日期" json:"refundDate"`
}

type RefundStatus int

const (
	RefundStatusPending   RefundStatus = 0 // 待退款
	RefundStatusProcessed RefundStatus = 1 // 退款处理中
	RefundStatusCompleted RefundStatus = 2 // 退款完成
	RefundStatusFailed    RefundStatus = 3 // 退款失败
)

// 退款订单项
type RefundOrderItem struct {
	*powermodel.PowerModel

	RefundOrder *RefundOrder `gorm:"foreignKey:RefundOrderId;references:Id" json:"order"`

	// 退款项信息
	RefundOrderId int64        `gorm:"comment:退款订单Id; index" json:"refundOrderId"`
	RefundNumber  string       `gorm:"comment:退款订单号; index" json:"refundNumber"`
	RefundStatus  RefundStatus `gorm:"comment:退款状态" json:"refundStatus"`
	RefundAmount  float64      `gorm:"type:decimal(10,2); comment:退款金额" json:"refundAmount"`
	RefundDate    time.Time    `gorm:"comment:退款日期" json:"refundDate"`
}
