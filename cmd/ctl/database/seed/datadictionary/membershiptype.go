package datadictionary

import (
	"PowerX/internal/model"
)

func defaultMembershipTypeDataDictionary() *model.DataDictionaryType {
	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   operation.MembershipTypeBase,
				Type:  operation.TypeMembershipType,
				Name:  "基本会籍",
				Value: operation.MembershipTypeBase,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   operation.MembershipTypeNormal,
				Type:  operation.TypeMembershipType,
				Name:  "普通会籍",
				Value: operation.MembershipTypeNormal,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   operation.MembershipTypePremium,
				Type:  operation.TypeMembershipType,
				Name:  "高级会籍",
				Value: operation.MembershipTypePremium,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   operation.MembershipTypeVIP,
				Type:  operation.TypeMembershipType,
				Name:  "VIP会籍",
				Value: operation.MembershipTypeVIP,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   operation.MembershipTypeCustom,
				Type:  operation.TypeMembershipType,
				Name:  "定制会籍",
				Value: operation.MembershipTypeCustom,
				Sort:  0,
			},
		},
		Type:        operation.TypeMembershipType,
		Name:        "会籍类型",
		Description: "会籍的类型区分",
	}

}
