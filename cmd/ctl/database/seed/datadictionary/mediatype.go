package datadictionary

import (
	"PowerX/internal/model"
	"PowerX/internal/model/market"
)

func defaultMediaTypeDataDictionary() *model.DataDictionaryType {
	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   market.MediaTypeProductShowcase,
				Type:  market.TypeMediaType,
				Name:  "产品展示",
				Value: market.MediaTypeProductShowcase,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   market.MediaTypeTutorialDemo,
				Type:  market.TypeMediaType,
				Name:  "教程和演示",
				Value: market.MediaTypeTutorialDemo,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   market.MediaTypeCustomerReviews,
				Type:  market.TypeMediaType,
				Name:  "买家评价",
				Value: market.MediaTypeCustomerReviews,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   market.MediaTypeBrandStory,
				Type:  market.TypeMediaType,
				Name:  "品牌故事",
				Value: market.MediaTypeBrandStory,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   market.MediaTypePromotionalCampaigns,
				Type:  market.TypeMediaType,
				Name:  "促销活动",
				Value: market.MediaTypePromotionalCampaigns,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   market.MediaTypeSocialMediaPromotion,
				Type:  market.TypeMediaType,
				Name:  "社交媒体推广",
				Value: market.MediaTypeSocialMediaPromotion,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   market.MediaTypeTrialSamples,
				Type:  market.TypeMediaType,
				Name:  "试用和样品",
				Value: market.MediaTypeTrialSamples,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   market.MediaTypeRecommendations,
				Type:  market.TypeMediaType,
				Name:  "推荐和搭配",
				Value: market.MediaTypeRecommendations,
				Sort:  0,
			},

			&model.DataDictionaryItem{
				Key:   market.MediaTypeUserGeneratedContent,
				Type:  market.TypeMediaType,
				Name:  "用户生成内容",
				Value: market.MediaTypeUserGeneratedContent,
				Sort:  0,
			},
		},
		Type:        market.TypeMediaType,
		Name:        "媒体类型",
		Description: "媒体的类型区分",
	}

}
