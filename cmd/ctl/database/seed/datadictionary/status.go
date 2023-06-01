package datadictionary

import "PowerX/internal/model"

func defaultStatusDataDictionary() *model.DataDictionaryType {
	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   model.StatusDraft,
				Type:  model.TypeApprovalStatus,
				Name:  "草稿",
				Value: model.StatusDraft,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.StatusActive,
				Type:  model.TypeApprovalStatus,
				Name:  "激活",
				Value: model.StatusActive,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.StatusCanceled,
				Type:  model.TypeApprovalStatus,
				Name:  "取消",
				Value: model.StatusCanceled,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.StatusPending,
				Type:  model.TypeApprovalStatus,
				Name:  "代办",
				Value: model.StatusPending,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.StatusInactive,
				Type:  model.TypeApprovalStatus,
				Name:  "无效",
				Value: model.StatusInactive,
				Sort:  0,
			},
		},
		Type:        model.TypeObjectStatus,
		Name:        "对象状态",
		Description: "默认对象的生命状态",
	}

}
