package market

import (
	"PowerX/internal/model"
	"PowerX/internal/model/powermodel"
)

type PromotionRule struct {
	powermodel.PowerModel

	MinPurchase float64 `gorm:"comment:起订购买量" json:"minPurchase"`
	BonusAmount float64 `gorm:"comment:额外赠送" json:"bonusAmount"`
}

func (mdl *PromotionRule) TableName() string {
	return model.PowerXSchema + "." + model.TableNamePromotionRule
}

func (mdl *PromotionRule) GetTableName(needFull bool) string {
	tableName := model.TableNamePromotionRule
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}
