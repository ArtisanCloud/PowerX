package datadictionary

import (
	"PowerX/internal/model"
)

func defaultMembershipStatusDataDictionary() *model.DataDictionaryType {
	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   operation.MembershipStatusActive,
				Type:  operation.TypeMembershipStatus,
				Name:  "活跃状态",
				Value: operation.MembershipStatusActive,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   operation.MembershipStatusInactive,
				Type:  operation.TypeMembershipStatus,
				Name:  "非活跃状态",
				Value: operation.MembershipStatusInactive,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   operation.MembershipStatusExpired,
				Type:  operation.TypeMembershipStatus,
				Name:  "已过期",
				Value: operation.MembershipStatusExpired,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   operation.MembershipStatusCancelled,
				Type:  operation.TypeMembershipStatus,
				Name:  "已取消",
				Value: operation.MembershipStatusCancelled,
				Sort:  0,
			},
		},
		Type:        operation.TypeMembershipStatus,
		Name:        "会籍状态",
		Description: "会籍状态区分",
	}
}
