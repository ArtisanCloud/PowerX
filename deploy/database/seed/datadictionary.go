package seed

import (
	"PowerX/internal/model"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func CreateDataDictionary(db *gorm.DB) (err error) {

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
					Key:   "wechat",
					Type:  "sales_platform",
					Name:  "微信",
					Value: "wechat",
					Sort:  0,
				},
				&model.DataDictionaryItem{
					Key:   "dianpin",
					Type:  "sales_platform",
					Name:  "点评网",
					Value: "dianpin",
					Sort:  0,
				},
				&model.DataDictionaryItem{
					Key:   "meituan",
					Type:  "sales_platform",
					Name:  "美团",
					Value: "meituan",
					Sort:  0,
				},
			},
			Type:        "sales_platform",
			Name:        "销售平台",
			Description: "可以在哪些平台上销售",
		},
		&model.DataDictionaryType{
			Items: []*model.DataDictionaryItem{
				&model.DataDictionaryItem{
					Key:   "wechat",
					Type:  "promote_platform",
					Name:  "微信",
					Value: "wechat",
					Sort:  0,
				},
				&model.DataDictionaryItem{
					Key:   "dianpin",
					Type:  "promote_platform",
					Name:  "点评网",
					Value: "dianpin",
					Sort:  0,
				},
				&model.DataDictionaryItem{
					Key:   "meituan",
					Type:  "promote_platform",
					Name:  "美团",
					Value: "meituan",
					Sort:  0,
				},
				&model.DataDictionaryItem{
					Key:   "douyin",
					Type:  "promote_platform",
					Name:  "抖音",
					Value: "douyin",
					Sort:  0,
				},
			},
			Type:        "promote_platform",
			Name:        "推广平台",
			Description: "可以在哪些平台上推广",
		},
		&model.DataDictionaryType{
			Items: []*model.DataDictionaryItem{
				&model.DataDictionaryItem{
					Key:   "wechat",
					Type:  "source_channel",
					Name:  "微信",
					Value: "wechat",
					Sort:  0,
				},
				&model.DataDictionaryItem{
					Key:   "dianpin",
					Type:  "source_channel",
					Name:  "点评网",
					Value: "dianpin",
					Sort:  0,
				},
				&model.DataDictionaryItem{
					Key:   "meituan",
					Type:  "source_channel",
					Name:  "美团",
					Value: "meituan",
					Sort:  0,
				},
				&model.DataDictionaryItem{
					Key:   "douyin",
					Type:  "source_channel",
					Name:  "抖音",
					Value: "douyin",
					Sort:  0,
				},
			},
			Type:        "source_channel",
			Name:        "公域渠道",
			Description: "获取公域流量的来源",
		},
	}

	return data

}
