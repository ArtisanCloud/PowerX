package reservationcenter

import (
	"PowerX/internal/model/powermodel"
	"time"
)

type Schedule struct {
	powermodel.PowerModel

	//Channel *model.DataDictionary `gorm:"comment:渠道"`

	CustomerId int64 `gorm:"comment:客户Id"`

	ApprovalStatus     string    `gorm:"comment:审批状态" json:"approvalStatus"`
	Capacity           int32     `gorm:"comment:最大客服服务容量" json:"capacity"`
	CopyFromScheduleId int64     `gorm:"comment:复制从日程表Id" json:"copyFromScheduleId"`
	Name               string    `gorm:"comment:名字" json:"name"`
	Description        string    `gorm:"comment:描述" json:"description"`
	IsActive           string    `gorm:"comment:开放状态" json:"isActive"`
	Status             string    `gorm:"comment:记录状态" json:"status"`
	StoreId            int64     `gorm:"comment:店铺Id" json:"storeId"`
	StartTime          time.Time `gorm:"comment:开始时间" json:"startTime"`
	EndTime            time.Time `gorm:"comment:结束时间" json:"endTime"`
}

const ScheduleUniqueId = powermodel.UniqueId
