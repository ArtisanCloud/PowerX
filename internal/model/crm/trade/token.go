package trade

import (
	"PowerX/internal/model/crm/customerdomain"
	"PowerX/internal/model/powermodel"
)

type TokenExchangeRatio struct {
	powermodel.PowerModel

	FromCategory int     `gorm:"comment:要兑换的代币种类" json:"fromCategory"`
	Ratio        float64 `gorm:"comment:兑换比例" json:"ratio"`
}

type TokenBalance struct {
	powermodel.PowerModel

	Customer *customerdomain.Customer `gorm:"foreignKey:CustomerId;references:Id" json:"customer"`

	CustomerId int64   `gorm:"comment:客户Id; index" json:"customerId"`
	Balance    float64 `gorm:"comment:代币余额" json:"balance"`
	Usage      float64 `gorm:"comment:使用代币 " json:"usage"`
}

const TokenBalanceUniqueId = powermodel.UniqueId
const TokenExchangeRecordId = powermodel.UniqueId

type TokenExchangeRecord struct {
	powermodel.PowerModel

	CustomerId     int64   `gorm:"comment:客户Id; index" json:"customerId"`
	ProductId      int64   `gorm:"comment:产品Id" json:"productId"`
	OrderId        int64   `gorm:"comment:订单Id" json:"orderId"`
	SourceCategory int     `gorm:"comment:原品种; index" json:"sourceCategory"`
	SourceAmount   float64 `gorm:"comment:原金额" json:"sourceAmount"`
	TokenAmount    float64 `gorm:"comment:换代币金额" json:"tokenAmount"`
	ExchangeRateId int64   `gorm:"comment:兑换比例Id" json:"exchangeRateId"`
	ExchangeRate   float64 `gorm:"comment:兑换比例" json:"exchangeRate"`
}

// TokenCategory 代表代币的种类
const TypeTokenCategory = "_token_category"

const (
	TokenCategoryPurchase = "_purchase" // 用于购买的代币
	TokenCategoryTask     = "_task"     // 任务奖励的代币
	TokenCategoryMember   = "_member"   // 会员特权的代币
	TokenCategoryReferral = "_referral" // 推荐奖励的代币
	TokenCategorySocial   = "_social"   // 社交互动的代币
	TokenCategoryCoupon   = "_coupon"   // 优惠券兑换的代币
	TokenCategoryCharity  = "_charity"  // 慈善捐赠的代币
	// 添加其他种类...
)

// 定义预扣记录对象
type TokenReservation struct {
	powermodel.PowerModel
	CustomerId  int64   `gorm:"comment:客户ID; index" json:"customerId"`
	Amount      float64 `gorm:"comment:预扣金额" json:"amount"`
	SourceType  string  `gorm:"column:source_type; not null;index:idx_src_type;comment:来源对象表名称" json:"sourceOwner"`
	SourceID    int64   `gorm:"column:source_id; not null;index:idx_src_id;comment:对象Id" json:"sourceId"`
	IsConfirmed bool    `gorm:"comment:是否已确认" json:"isConfirmed"`
}

// 定义交易记录对象
type TokenTransaction struct {
	powermodel.PowerModel

	CustomerId int64   `gorm:"comment:客户ID; index" json:"customerId"`
	Amount     float64 `gorm:"comment:交易金额" json:"amount"`
	Type       int     `gorm:"comment:交易类型（增加或减少）" json:"transactionType"`
	SourceType string  `gorm:"column:source_type; not null;index:idx_src_type;comment:来源对象表名称" json:"sourceOwner"`
	SourceID   int64   `gorm:"column:source_id; not null;index:idx_src_id;comment:对象Id" json:"sourceId"`
}

const TokenTransactionId = powermodel.UniqueId

const TypeTokenTransactionType = "_token_transaction_type"

const (
	TokenTransactionTypePurchase = "_purchase" // 购买
	TokenTransactionTypeReward   = "_reward"   // 奖励
	TokenTransactionTypeExchange = "_exchange" // 兑换
	TokenTransactionTypeSpending = "_spending" // 消费
	TokenTransactionTypeRefund   = "_refund"   // 退款
	TokenTransactionTypeExpired  = "_expired"  // 过期
	TokenTransactionTypeGift     = "_gift"     // 赠送
	TokenTransactionTypeInterest = "_interest" // 利息
	TokenTransactionTypeInvest   = "_invest"   // 投资
	TokenTransactionTypeOther    = "_other"    // 其他
)

func ConvertTokens(fromCategory, amount float64) float64 {

	return 0.0
}
