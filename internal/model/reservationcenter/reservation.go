package reservationcenter

import (
	"PowerX/internal/model/powermodel"
	"time"
)

type Reservation struct {
	powermodel.PowerModel

	//Channel *model.DataDictionary `gorm:"comment:渠道"`

	CustomerId          int64     `gorm:"comment:客户Id"`
	ConsumedPoints      float32   `gorm:"comment:消耗点数，非必填"`
	ReservedTime        time.Time `gorm:"comment:已预约时间"`
	CancelTime          time.Time `gorm:"comment:取消时间"`
	CheckinTime         time.Time `gorm:"comment:签到时间"`
	ConsumedDate        time.Time `gorm:"comment:抵扣点数时间"`
	Description         string    `gorm:"comment:描述"`
	ConsumeMembershipId int64     `gorm:"comment:抵扣会籍Id"`
	Name                string    `gorm:"comment:预约记录名称"`
	Type                int8      `gorm:"comment:类型，包括在线，线下，电话等"`
	OperationStatus     int8      `gorm:"comment:操作状态"`
	ReservationStatus   int8      `gorm:"comment:预约状态"`
	ReservedArtisanId   int64     `gorm:"comment:预约的设计师Id"`
}

const ReservationUniqueId = powermodel.UniqueId

const ReservationTypeOnSite = 1
const ReservationTypeOnline = 2
const ReservationTypePhone = 3

const OperationStatusNone = 1
const OperationStatusCancelling = 2
const OperationStatusCancelled = 3
const OperationStatusLateCancelled = 4
const OperationStatusNoShow = 5
const OperationStatusCheckIn = 6
const OperationStatusCancelFailed = 7
const OperationStatusAutoCancelled = 8
const OperationStatusReservedPlace = 9

const ReservationStatusDraft = 1
const ReservationStatusConfirmed = 2
const ReservationStatusCancelled = 3
const ReservationStatusFailed = 4
