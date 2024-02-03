package datadictionary

import (
	"PowerX/internal/model"
	"PowerX/internal/model/crm/trade"
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

func defaultTokenTransactionDataDictionary() *model.DataDictionaryType {
	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   trade.TokenTransactionTypePurchase,
				Type:  trade.TypeTokenTransactionType,
				Name:  "购买",
				Value: trade.TokenTransactionTypePurchase,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.TokenTransactionTypeReward,
				Type:  trade.TypeTokenTransactionType,
				Name:  "奖励",
				Value: trade.TokenTransactionTypeReward,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.TokenTransactionTypeExchange,
				Type:  trade.TypeTokenTransactionType,
				Name:  "兑换",
				Value: trade.TokenTransactionTypeExchange,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.TokenTransactionTypeSpending,
				Type:  trade.TypeTokenTransactionType,
				Name:  "消费",
				Value: trade.TokenTransactionTypeSpending,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.TokenTransactionTypeRefund,
				Type:  trade.TypeTokenTransactionType,
				Name:  "退款",
				Value: trade.TokenTransactionTypeRefund,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.TokenTransactionTypeExpired,
				Type:  trade.TypeTokenTransactionType,
				Name:  "过期",
				Value: trade.TokenTransactionTypeExpired,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.TokenTransactionTypeGift,
				Type:  trade.TypeTokenTransactionType,
				Name:  "赠送",
				Value: trade.TokenTransactionTypeGift,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.TokenTransactionTypeInterest,
				Type:  trade.TypeTokenTransactionType,
				Name:  "利息",
				Value: trade.TokenTransactionTypeInterest,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.TokenTransactionTypeInvest,
				Type:  trade.TypeTokenTransactionType,
				Name:  "投资",
				Value: trade.TokenTransactionTypeInvest,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.TokenTransactionTypeOther,
				Type:  trade.TypeTokenTransactionType,
				Name:  "其他",
				Value: trade.TokenTransactionTypeOther,
				Sort:  0,
			},
		},
		Type:        trade.TypeTokenTransactionType,
		Name:        "代币的流水",
		Description: "代币的使用交易流水记录",
	}
}
