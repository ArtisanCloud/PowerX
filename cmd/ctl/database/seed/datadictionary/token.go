package datadictionary

import (
	"PowerX/internal/model"
	"PowerX/internal/model/trade"
)

func defaultTokenCategoryDataDictionary() *model.DataDictionaryType {
	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   trade.TokenCategoryPurchase,
				Type:  trade.TypeTokenCategory,
				Name:  "用于购买的代币",
				Value: trade.TokenCategoryPurchase,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.TokenCategoryTask,
				Type:  trade.TypeTokenCategory,
				Name:  "任务奖励的代币",
				Value: trade.TokenCategoryTask,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.TokenCategoryMember,
				Type:  trade.TypeTokenCategory,
				Name:  "会员特权的代币",
				Value: trade.TokenCategoryMember,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.TokenCategoryReferral,
				Type:  trade.TypeTokenCategory,
				Name:  "推荐奖励的代币",
				Value: trade.TokenCategoryReferral,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.TokenCategorySocial,
				Type:  trade.TypeTokenCategory,
				Name:  "社交互动的代币",
				Value: trade.TokenCategorySocial,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.TokenCategoryCoupon,
				Type:  trade.TypeTokenCategory,
				Name:  "优惠券兑换的代币",
				Value: trade.TokenCategoryCoupon,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.TokenCategoryCharity,
				Type:  trade.TypeTokenCategory,
				Name:  "慈善捐赠的代币",
				Value: trade.TokenCategoryCharity,
				Sort:  0,
			},
		},
		Type:        trade.TypeTokenCategory,
		Name:        "代币的种类",
		Description: "代表代币的种类",
	}

}
