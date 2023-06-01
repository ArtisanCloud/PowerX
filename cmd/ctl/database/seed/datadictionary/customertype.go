package datadictionary

import (
	"PowerX/internal/model"
	"PowerX/internal/model/customerdomain"
)

func defaultCustomerTypeDataDictionary() *model.DataDictionaryType {
	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   customerdomain.CustomerPersonal,
				Type:  customerdomain.TypeCustomerType,
				Name:  "个人",
				Value: customerdomain.CustomerPersonal,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   customerdomain.CustomerCompany,
				Type:  customerdomain.TypeCustomerType,
				Name:  "公司",
				Value: customerdomain.CustomerCompany,
				Sort:  0,
			},
		},
		Type:        customerdomain.TypeCustomerType,
		Name:        "客户类型",
		Description: "客户类型分个人，公司",
	}
}
