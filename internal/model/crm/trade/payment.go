package trade

import (
	"PowerX/internal/model"
	"PowerX/internal/model/powermodel"
	"github.com/ArtisanCloud/PowerLibs/v3/object"
	"github.com/golang-module/carbon/v2"
	"time"
)

type Payment struct {
	*powermodel.PowerModel

	Order *Order         `gorm:"foreignKey:OrderId;references:Id" json:"order"`
	Items []*PaymentItem `gorm:"foreignKey:PaymentID;references:Id" json:"items"`

	OrderId         int64     `gorm:"comment:订单Id" json:"orderId"`
	PaymentDate     time.Time `gorm:"comment:支付日期" json:"paymentDate"`
	PaymentType     int       `gorm:"comment:支付方式" json:"paymentType"`
	PaidAmount      float64   `gorm:"type:decimal(10,2); comment:实际支付金额" json:"paidAmount"`
	PaymentNumber   string    `gorm:"comment:支付单单号" json:"paymentNumber"`
	ReferenceNumber string    `gorm:"comment:参考单号" json:"referenceNumber"`
	Remark          string    `gorm:"comment:备注" json:"remark"`
	Status          int       `gorm:"comment:支付单状态" json:"status"`
}

func (mdl *Payment) TableName() string {
	return model.PowerXSchema + "." + model.TableNamePayment
}

func (mdl *Payment) GetTableName(needFull bool) string {
	tableName := model.TableNamePayment
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}

const TypePaymentType = "_payment_type"
const TypePaymentStatus = "_payment_status"

const (
	PaymentStatusPending   = "_pending"   // 待支付
	PaymentStatusPaid      = "_paid"      // 已支付
	PaymentStatusRefunded  = "_refunded"  // 已退款
	PaymentStatusCancelled = "_cancelled" // 已取消
)

const (
	PaymentTypeBank       = "_bank"        // 银行
	PaymentTypeWeChat     = "_wechat"      // 微信
	PaymentTypeAlipay     = "_alipay"      // 支付宝
	PaymentTypePayPal     = "_paypal"      // PayPal
	PaymentTypeCreditCard = "_credit_card" // 信用卡
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

const PaymentUniqueId = powermodel.UniqueId

func (mdl *PaymentItem) TableName() string {
	return model.PowerXSchema + "." + model.TableNamePaymentItem
}

func (mdl *PaymentItem) GetTableName(needFull bool) string {
	tableName := model.TableNamePaymentItem
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}

func GeneratePaymentNumber() string {
	return "PO" + carbon.Now().Format("YmdHis") + object.QuickRandom(6)
}

//func (mdl *Payment) IsStatusToBePaid() bool {
//	return mdl.Status == PaymentStatusPending
//}
