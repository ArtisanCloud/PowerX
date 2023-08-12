package datadictionary

import (
	"PowerX/internal/model"
	"PowerX/internal/model/trade"
)

func defaultOrderTypeDataDictionary() *model.DataDictionaryType {
	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   trade.OrderTypeNormal,
				Type:  trade.TypeOrderType,
				Name:  "普通订单",
				Value: trade.OrderTypeNormal,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.OrderTypePreorder,
				Type:  trade.TypeOrderType,
				Name:  "预定订单",
				Value: trade.OrderTypePreorder,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.OrderTypeCart,
				Type:  trade.TypeOrderType,
				Name:  "购物车订单",
				Value: trade.OrderTypeCart,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.OrderTypeCustom,
				Type:  trade.TypeOrderType,
				Name:  "定制订单",
				Value: trade.OrderTypeCustom,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.OrderTypeSubscription,
				Type:  trade.TypeOrderType,
				Name:  "订阅订单",
				Value: trade.OrderTypeSubscription,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.OrderTypeWholesale,
				Type:  trade.TypeOrderType,
				Name:  "批发订单",
				Value: trade.OrderTypeWholesale,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.OrderTypeGift,
				Type:  trade.TypeOrderType,
				Name:  "赠品订单",
				Value: trade.OrderTypeGift,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.OrderTypeBonus,
				Type:  trade.TypeOrderType,
				Name:  "奖励订单",
				Value: trade.OrderTypeBonus,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.OrderTypeGiftWithPurchase,
				Type:  trade.TypeOrderType,
				Name:  "有赠品的订单",
				Value: trade.OrderTypeGiftWithPurchase,
				Sort:  0,
			},

			&model.DataDictionaryItem{
				Key:   trade.OrderTypeReturn,
				Type:  trade.TypeOrderType,
				Name:  "退货订单",
				Value: trade.OrderTypeReturn,
				Sort:  0,
			},

			&model.DataDictionaryItem{
				Key:   trade.OrderTypeExchange,
				Type:  trade.TypeOrderType,
				Name:  "换货订单",
				Value: trade.OrderTypeExchange,
				Sort:  0,
			},

			&model.DataDictionaryItem{
				Key:   trade.OrderTypeReshipment,
				Type:  trade.TypeOrderType,
				Name:  "补发订单",
				Value: trade.OrderTypeReshipment,
				Sort:  0,
			},
		},
		Type:        trade.TypeOrderType,
		Name:        "订单类型",
		Description: "订单的类型区分",
	}

}
