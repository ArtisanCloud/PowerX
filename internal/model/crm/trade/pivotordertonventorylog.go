package trade

import (
	"PowerX/internal/model/powermodel"
)

type PivotProductToPromotion struct {
	*powermodel.PowerPivot

	PromotionRuleId int64 `gorm:"comment:促销规则d; not null;index:idx_promotion_rule_id" json:"promotionRuleId"`
	ProductId       int64 `gorm:"comment:商品Id; not null;index:idx_product_id" json:"productId"`
}
