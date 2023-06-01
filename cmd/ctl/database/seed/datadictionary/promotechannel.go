package datadictionary

import "PowerX/internal/model"

func defaultPromoteChannelsDataDictionary() *model.DataDictionaryType {
	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   model.ChannelWechat,
				Type:  model.TypePromoteChannel,
				Name:  "微信",
				Value: model.ChannelWechat,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ChannelDianPing,
				Type:  model.TypePromoteChannel,
				Name:  "点评网",
				Value: model.ChannelDianPing,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ChannelMeiTuan,
				Type:  model.TypePromoteChannel,
				Name:  "美团",
				Value: model.ChannelMeiTuan,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ChannelDouYin,
				Type:  model.TypePromoteChannel,
				Name:  "抖音",
				Value: model.ChannelDouYin,
				Sort:  0,
			},
		},
		Type:        model.TypePromoteChannel,
		Name:        "推广平台",
		Description: "可以在哪些平台上推广",
	}
}
