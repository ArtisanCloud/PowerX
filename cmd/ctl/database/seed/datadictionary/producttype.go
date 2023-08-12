package datadictionary

import (
	"PowerX/internal/model"
	"PowerX/internal/model/product"
)

func defaultProductTypeDataDictionary() *model.DataDictionaryType {
	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   product.ProductTypeToken,
				Type:  product.TypeProductType,
				Name:  "代币品类",
				Value: product.ProductTypeToken,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   product.ProductTypeGoods,
				Type:  product.TypeProductType,
				Name:  "普通商品",
				Value: product.ProductTypeGoods,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   product.ProductTypeService,
				Type:  product.TypeProductType,
				Name:  "周期性商品",
				Value: product.ProductTypeService,
				Sort:  0,
			},
		},
		Type:        product.TypeProductType,
		Name:        "产品计划",
		Description: "产品类型分实体商品，虚拟产品",
	}

}
