package datadictionary

import "PowerX/internal/model"

func defaultSalesChannelsDataDictionary() *model.DataDictionaryType {
	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   model.ChannelWechat,
				Type:  model.TypeSalesChannel,
				Name:  "微信",
				Value: model.ChannelWechat,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ChannelTaoBao,
				Type:  model.TypeSalesChannel,
				Name:  "淘宝",
				Value: model.ChannelTaoBao,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ChannelJD,
				Type:  model.TypeSalesChannel,
				Name:  "京东",
				Value: model.ChannelJD,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ChannelDianPing,
				Type:  model.TypeSalesChannel,
				Name:  "点评网",
				Value: model.ChannelDianPing,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ChannelMeiTuan,
				Type:  model.TypeSalesChannel,
				Name:  "美团",
				Value: model.ChannelMeiTuan,
				Sort:  0,
			},
		},
		Type:        model.TypeSalesChannel,
		Name:        "销售平台",
		Description: "可以在哪些平台上销售",
	}
}
