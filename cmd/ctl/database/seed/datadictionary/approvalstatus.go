package datadictionary

import "PowerX/internal/model"

func defaultApprovalStatusDataDictionary() *model.DataDictionaryType {
	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   model.ApprovalStatusApply,
				Type:  model.TypeApprovalStatus,
				Name:  "待审核",
				Value: model.ApprovalStatusApply,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ApprovalStatusReject,
				Type:  model.TypeApprovalStatus,
				Name:  "拒绝",
				Value: model.ApprovalStatusReject,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ApprovalStatusSuccess,
				Type:  model.TypeApprovalStatus,
				Name:  "通过",
				Value: model.ApprovalStatusSuccess,
				Sort:  0,
			},
		},
		Type:        model.TypeApprovalStatus,
		Name:        "审核状态",
		Description: "可以在哪些平台上销售",
	}
}
