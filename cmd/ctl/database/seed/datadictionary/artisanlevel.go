package datadictionary

import (
	"PowerX/internal/model"
	"PowerX/internal/model/product"
)

func defaultArtisanLevelDataDictionary() *model.DataDictionaryType {
	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   product.ArtisanLevelBasic,
				Type:  product.ArtisanLevelType,
				Name:  "初级",
				Value: product.ArtisanLevelBasic,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   product.ArtisanLevelMedium,
				Type:  product.ArtisanLevelType,
				Name:  "中级",
				Value: product.ArtisanLevelMedium,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   product.ArtisanLevelAdvanced,
				Type:  product.ArtisanLevelType,
				Name:  "高级",
				Value: product.ArtisanLevelAdvanced,
				Sort:  0,
			},
		},
		Type:        product.ArtisanLevelType,
		Name:        "Artisan等级",
		Description: "Artisan的等级区分",
	}

}
