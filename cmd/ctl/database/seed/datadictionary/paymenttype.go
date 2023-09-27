package datadictionary

import (
	"PowerX/internal/model"
	"PowerX/internal/model/crm/trade"
)

func defaultPaymentTypeDataDictionary() *model.DataDictionaryType {
	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   trade.PaymentTypeBank,
				Type:  trade.TypePaymentType,
				Name:  "银行",
				Value: trade.PaymentTypeBank,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.PaymentTypeWeChat,
				Type:  trade.TypePaymentType,
				Name:  "微信",
				Value: trade.PaymentTypeWeChat,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.PaymentTypeAlipay,
				Type:  trade.TypePaymentType,
				Name:  "支付宝",
				Value: trade.PaymentTypeAlipay,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.PaymentTypePayPal,
				Type:  trade.TypePaymentType,
				Name:  "PayPal",
				Value: trade.PaymentTypePayPal,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   trade.PaymentTypeCreditCard,
				Type:  trade.TypePaymentType,
				Name:  "信用卡",
				Value: trade.PaymentTypeCreditCard,
				Sort:  0,
			},
		},
		Type:        trade.TypePaymentType,
		Name:        "支付单类型",
		Description: "支付单的类型区分",
	}

}
