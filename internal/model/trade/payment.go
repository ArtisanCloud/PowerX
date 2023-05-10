package trade

import (
	"PowerX/internal/model/powermodel"
	"time"
)

type Payment struct {
	*powermodel.PowerModel

	Order *Order `gorm:"foreignKey:OrderId;references:Id" json:"order"`

	OrderId             int64     `gorm:"comment:订单Id" json:"orderId"`
	PaymentDate         time.Time `gorm:"comment:支付日期" json:"paymentDate"`
	PaymentType         int       `gorm:"comment:支付方式" json:"paymentType"`
	PaidAmount          float64   `gorm:"type:decimal(10,2); comment:实际支付金额" json:"paidAmount"`
	PaymentNumber       string    `gorm:"comment:支付单单号" json:"paymentNumber"`
	ReferenceNumber     string    `gorm:"comment:参考单号" json:"referenceNumber"`
	Status              int       `gorm:"comment:支付单状态" json:"status"`
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
