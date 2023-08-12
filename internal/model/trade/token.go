package trade

import (
	"PowerX/internal/model/customerdomain"
	"PowerX/internal/model/powermodel"
	"github.com/ArtisanCloud/PowerLibs/v3/database"
)

type ExchangeRatio struct {
	database.PowerModel

	FromCategory int     `gorm:"comment:要兑换的代币种类" json:"fromCategory"`
	Ratio        float64 `gorm:"comment:兑换比例" json:"ratio"`
}

type TokenBalance struct {
	database.PowerModel

	Customer *customerdomain.Customer `gorm:"foreignKey:CustomerId;references:Id" json:"customer"`

	CustomerId int64   `gorm:"comment:客户Id; index" json:"customerId"`
	Balance    float64 `gorm:"comment:代币余额; index" json:"balance"`
}

const TokenBalanceUniqueId = powermodel.UniqueId

type ExchangeRecord struct {
	database.PowerModel

	CustomerId     int64   `gorm:"comment:客户Id; index" json:"customerId"`
	SourceCategory int     `gorm:"comment:原品种; index" json:"sourceCategory"`
	SourceAmount   float64 `gorm:"comment:原金额; index" json:"sourceAmount"`
	TokenAmount    float64 `gorm:"comment:换代币金额; index" json:"tokenAmount"`
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

func ConvertTokens(fromCategory, amount float64) float64 {

	return 0.0
}
