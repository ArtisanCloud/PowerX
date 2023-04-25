package seed

import (
	"PowerX/internal/model"
	"PowerX/internal/model/custom/reservationcenter"
)

func CustomDataDictionary() (data []*model.DataDictionaryType) {

	data = []*model.DataDictionaryType{
		operationStatusDataDictionary(),
		reservationTypesDataDictionary(),
		reservationStatusDataDictionary(),
	}

	return data

}

func operationStatusDataDictionary() *model.DataDictionaryType {

	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   reservationcenter.OperationStatusNone,
				Type:  reservationcenter.OperationStatusType,
				Name:  "无操作",
				Value: reservationcenter.OperationStatusNone,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   reservationcenter.OperationStatusCancelling,
				Type:  reservationcenter.OperationStatusType,
				Name:  "取消中",
				Value: reservationcenter.OperationStatusCancelling,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   reservationcenter.OperationStatusCancelled,
				Type:  reservationcenter.OperationStatusType,
				Name:  "已取消",
				Value: reservationcenter.OperationStatusCancelled,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   reservationcenter.OperationStatusCancelFailed,
				Type:  reservationcenter.OperationStatusType,
				Name:  "取消失败",
				Value: reservationcenter.OperationStatusCancelFailed,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   reservationcenter.OperationStatusLateCancelled,
				Type:  reservationcenter.OperationStatusType,
				Name:  "事后取消",
				Value: reservationcenter.OperationStatusLateCancelled,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   reservationcenter.OperationStatusAutoCancelled,
				Type:  reservationcenter.OperationStatusType,
				Name:  "自动取消",
				Value: reservationcenter.OperationStatusAutoCancelled,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   reservationcenter.OperationStatusNoShow,
				Type:  reservationcenter.OperationStatusType,
				Name:  "未到场",
				Value: reservationcenter.OperationStatusNoShow,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   reservationcenter.OperationStatusCheckIn,
				Type:  reservationcenter.OperationStatusType,
				Name:  "到场",
				Value: reservationcenter.OperationStatusCheckIn,
				Sort:  0,
			},
		},
		Type:        reservationcenter.OperationStatusType,
		Name:        "预约操作状态",
		Description: "使用者操作预约单的操作状态",
	}

}

func reservationTypesDataDictionary() *model.DataDictionaryType {

	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   reservationcenter.ReservationTypeOnSite,
				Type:  reservationcenter.ReservationTypesType,
				Name:  "现场预约",
				Value: reservationcenter.ReservationTypeOnSite,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   reservationcenter.ReservationTypeOnline,
				Type:  reservationcenter.ReservationTypesType,
				Name:  "线上预约",
				Value: reservationcenter.ReservationTypeOnline,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   reservationcenter.ReservationTypePhone,
				Type:  reservationcenter.ReservationTypesType,
				Name:  "电话预约",
				Value: reservationcenter.ReservationTypePhone,
				Sort:  0,
			},
		},
		Type:        reservationcenter.ReservationTypesType,
		Name:        "预约类型",
		Description: "约单的类型",
	}

}

func reservationStatusDataDictionary() *model.DataDictionaryType {

	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   reservationcenter.ReservationStatusDraft,
				Type:  reservationcenter.ReservationStatusType,
				Name:  "状态草稿",
				Value: reservationcenter.ReservationStatusDraft,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   reservationcenter.ReservationStatusConfirmed,
				Type:  reservationcenter.ReservationStatusType,
				Name:  "预约状态成功",
				Value: reservationcenter.ReservationStatusConfirmed,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   reservationcenter.ReservationStatusCancelled,
				Type:  reservationcenter.ReservationStatusType,
				Name:  "预约状态取消",
				Value: reservationcenter.ReservationStatusCancelled,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   reservationcenter.ReservationStatusFailed,
				Type:  reservationcenter.ReservationStatusType,
				Name:  "预约状态失败",
				Value: reservationcenter.ReservationStatusFailed,
				Sort:  0,
			},
		},
		Type:        reservationcenter.ReservationStatusType,
		Name:        "约单的状态",
		Description: "约单的各种状态",
	}

}
