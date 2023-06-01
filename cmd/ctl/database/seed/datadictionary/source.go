package datadictionary

import "PowerX/internal/model"

func defaultSourceDataDictionary() *model.DataDictionaryType {

	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   model.ChannelDirect,
				Type:  model.TypeSourceChannel,
				Name:  "品牌自营",
				Value: model.ChannelDirect,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ChannelWechat,
				Type:  model.TypeSourceChannel,
				Name:  "微信",
				Value: model.ChannelWechat,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ChannelDianPing,
				Type:  model.TypeSourceChannel,
				Name:  "点评网",
				Value: model.ChannelDianPing,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ChannelMeiTuan,
				Type:  model.TypeSourceChannel,
				Name:  "美团",
				Value: model.ChannelMeiTuan,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ChannelDouYin,
				Type:  model.TypeSourceChannel,
				Name:  "抖音",
				Value: model.ChannelDouYin,
				Sort:  0,
			},
		},
		Type:        model.TypeSourceChannel,
		Name:        "公域渠道",
		Description: "获取公域流量的来源",
	}
}
