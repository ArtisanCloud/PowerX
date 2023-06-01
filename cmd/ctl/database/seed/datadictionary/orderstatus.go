package datadictionary

import (
	"PowerX/internal/model"
	"PowerX/internal/model/trade"
)

func defaultOrderStatusDataDictionary() *model.DataDictionaryType {
	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   trade.OrderStatusPending,
				Type:  trade.TypeOrderStatus,
				Name:  "待处理",
				Value: trade.OrderStatusPending,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.OrderStatusToBePaid,
				Type:  trade.TypeOrderStatus,
				Name:  "待付款",
				Value: trade.OrderStatusToBePaid,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.OrderStatusConfirmed,
				Type:  trade.TypeOrderStatus,
				Name:  "已确认",
				Value: trade.OrderStatusConfirmed,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.OrderStatusToBeShipped,
				Type:  trade.TypeOrderStatus,
				Name:  "待发货",
				Value: trade.OrderStatusToBeShipped,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.OrderStatusShipping,
				Type:  trade.TypeOrderStatus,
				Name:  "送货中",
				Value: trade.OrderStatusShipping,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.OrderStatusDelivered,
				Type:  trade.TypeOrderStatus,
				Name:  "已签收",
				Value: trade.OrderStatusDelivered,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.OrderStatusCompleted,
				Type:  trade.TypeOrderStatus,
				Name:  "已完成",
				Value: trade.OrderStatusCompleted,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.OrderStatusCancelled,
				Type:  trade.TypeOrderStatus,
				Name:  "已取消",
				Value: trade.OrderStatusCancelled,
				Sort:  0,
			},

			&model.DataDictionaryItem{
				Key:   trade.OrderStatusFailed,
				Type:  trade.TypeOrderStatus,
				Name:  "失败",
				Value: trade.OrderStatusFailed,
				Sort:  0,
			},

			&model.DataDictionaryItem{
				Key:   trade.OrderStatusRefunding,
				Type:  trade.TypeOrderStatus,
				Name:  "退款中",
				Value: trade.OrderStatusRefunding,
				Sort:  0,
			},

			&model.DataDictionaryItem{
				Key:   trade.OrderStatusRefunded,
				Type:  trade.TypeOrderStatus,
				Name:  "已退款",
				Value: trade.OrderStatusRefunded,
				Sort:  0,
			},

			&model.DataDictionaryItem{
				Key:   trade.OrderStatusReturned,
				Type:  trade.TypeOrderStatus,
				Name:  "已退货",
				Value: trade.OrderStatusRefunded,
				Sort:  0,
			},
		},
		Type:        trade.TypeOrderStatus,
		Name:        "订单状态",
		Description: "订单状态区分",
	}
}
