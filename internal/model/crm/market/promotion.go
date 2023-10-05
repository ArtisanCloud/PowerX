package market

import "PowerX/internal/model/powermodel"

type PromotionRule struct {
	powermodel.PowerModel

	MinPurchase float64 `gorm:"comment:起订购买量" json:"minPurchase"`
	BonusAmount float64 `gorm:"comment:额外赠送" json:"bonusAmount"`
}
