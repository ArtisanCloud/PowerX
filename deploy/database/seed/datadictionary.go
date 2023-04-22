package seed

import (
	"PowerX/internal/model"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func CreateDataDictionaries(db *gorm.DB) (err error) {

	var count int64
	if err = db.Model(&model.DataDictionaryType{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init data dictionary  failed"))
	}

	data := DefaultDataDictionary()

	if count == 0 {
		if err = db.Model(&model.DataDictionaryType{}).Create(data).Error; err != nil {
			panic(errors.Wrap(err, "init data dictionary failed"))
		}
	}

	return err
}

func DefaultDataDictionary() (data []*model.DataDictionaryType) {

	data = []*model.DataDictionaryType{
		&model.DataDictionaryType{
			Items: []*model.DataDictionaryItem{
				&model.DataDictionaryItem{
					Key:   model.ChannelWechat,
					Type:  model.TypeSalesChannel,
					Name:  "微信",
					Value: model.ChannelWechat,
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
					Name:  model.ChannelMeiTuan,
					Value: model.ChannelMeiTuan,
					Sort:  0,
				},
			},
			Type:        model.TypeSalesChannel,
			Name:        "销售平台",
			Description: "可以在哪些平台上销售",
		},
		&model.DataDictionaryType{
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
		},
		&model.DataDictionaryType{
			Items: []*model.DataDictionaryItem{
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
		},
	}

	return data

}
