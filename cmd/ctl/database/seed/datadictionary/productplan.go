package datadictionary

import (
	"PowerX/internal/model"
	"PowerX/internal/model/product"
)

func defaultProductPlanDataDictionary() *model.DataDictionaryType {
	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   product.ProductPlanOnce,
				Type:  product.TypeProductPlan,
				Name:  "实体商品",
				Value: product.ProductPlanOnce,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   product.ProductPlanPeriod,
				Type:  product.TypeProductPlan,
				Name:  "虚拟产品",
				Value: product.ProductPlanPeriod,
				Sort:  0,
			},
		},
		Type:        product.TypeProductPlan,
		Name:        "产品类型",
		Description: "产品类型分实体商品，虚拟产品",
	}
}
