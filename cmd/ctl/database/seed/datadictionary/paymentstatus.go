package datadictionary

import (
	"PowerX/internal/model"
	"PowerX/internal/model/crm/trade"
)

func defaultPaymentStatusDataDictionary() *model.DataDictionaryType {
	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   trade.PaymentStatusPending,
				Type:  trade.TypePaymentStatus,
				Name:  "待支付",
				Value: trade.PaymentStatusPending,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.PaymentStatusPaid,
				Type:  trade.TypePaymentStatus,
				Name:  "已支付",
				Value: trade.PaymentStatusPaid,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.PaymentStatusRefunded,
				Type:  trade.TypePaymentStatus,
				Name:  "已退款",
				Value: trade.PaymentStatusRefunded,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.PaymentStatusCancelled,
				Type:  trade.TypePaymentStatus,
				Name:  "已取消",
				Value: trade.PaymentStatusCancelled,
				Sort:  0,
			},
		},
		Type:        trade.TypePaymentStatus,
		Name:        "支付单状态",
		Description: "支付单状态区分",
	}
}
