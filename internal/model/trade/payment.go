package trade

import (
	"PowerX/internal/model/powermodel"
	"time"
)

type Payment struct {
	*powermodel.PowerModel

	Order *Order `gorm:"foreignKey:OrderId;references:Id" json:"order"`

	OrderId         int64         `gorm:"comment:订单Id" json:"orderId"`
	PaymentDate     time.Time     `gorm:"comment:支付日期" json:"paymentDate"`
	PaymentType     int           `gorm:"comment:支付方式" json:"paymentType"`
	PaidAmount      float64       `gorm:"type:decimal(10,2); comment:实际支付金额" json:"paidAmount"`
	PaymentNumber   string        `gorm:"comment:支付单单号" json:"paymentNumber"`
	ReferenceNumber string        `gorm:"comment:参考单号" json:"referenceNumber"`
	Status          PaymentStatus `gorm:"comment:支付单状态" json:"status"`
}

type PaymentStatus int8

const (
	PaymentStatusPending   PaymentStatus = 0 // 待支付
	PaymentStatusPaid      PaymentStatus = 1 // 已支付
	PaymentStatusRefunded  PaymentStatus = 2 // 已退款
	PaymentStatusCancelled PaymentStatus = 3 // 已取消
)

type PaymentItem struct {
	*powermodel.PowerModel

	PaymentID           int64     `gorm:"comment:支付记录ID" json:"paymentId"`
	Quantity            int       `gorm:"comment:商品数量" json:"quantity"`
	UnitPrice           float64   `gorm:"comment:商品单价" json:"unitPrice"`
	PaymentCustomerName string    `gorm:"comment:支付客户名称" json:"paymentCustomerName"`
	BankInformation     string    `gorm:"comment:银行信息" json:"bankInformation"`
	BankResponseCode    string    `gorm:"comment:银行反馈码" json:"bankResponseCode"`
	CarrierType         string    `gorm:"comment:运营商类型" json:"carrierType"`
	CreditCardNumber    string    `gorm:"comment:行用卡号码" json:"creditCardNumber"`
	DeductMembershipId  string    `gorm:"comment:抵扣会籍Id" json:"deductMembershipId"`
	DeductionPoint      int32     `gorm:"comment:抵扣点数" json:"deductionPoint"`
	InvoiceCreateTime   time.Time `gorm:"comment:发票创建时间" json:"invoiceCreateTime"`
	InvoiceNumber       string    `gorm:"comment:发票号码" json:"invoiceNumber"`
	InvoiceTotalAmount  float64   `gorm:"comment:发票开票金额" json:"invoiceTotalAmount"`
	TaxIdNumber         string    `gorm:"comment:税号" json:"taxIdNumber"`
}
