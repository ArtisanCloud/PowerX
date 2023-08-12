package market

import "github.com/ArtisanCloud/PowerLibs/v3/database"

type PromotionRule struct {
	database.PowerModel

	MinPurchase float64 `gorm:"comment:起订购买量" json:"minPurchase"`
	BonusAmount float64 `gorm:"comment:额外赠送" json:"bonusAmount"`
}
