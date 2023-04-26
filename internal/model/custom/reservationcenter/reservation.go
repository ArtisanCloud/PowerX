package reservationcenter

import (
	"PowerX/internal/model/powermodel"
	"time"
)

// data dictinary types
const OperationStatusType = "_operation_status"     // 预约操作字典类型
const ReservationTypesType = "_reservation_type"    // 预约类型字典类型
const ReservationStatusType = "_reservation_status" // 预约状态字典类型

// data dictinary items
const OperationStatusNone = "_none"                    // 无操作
const OperationStatusCancelling = "_cancelling"        // 取消中
const OperationStatusCancelled = "_cancelled"          // 已取消
const OperationStatusCancelFailed = "_cancel_failed"   // 取消失败
const OperationStatusLateCancelled = "_late_cancelled" // 事后取消
const OperationStatusAutoCancelled = "_auto_cancelled" // 自动取消
const OperationStatusNoShow = "_no_show"               // 未到场
const OperationStatusCheckIn = "_checkin"              // 到场

const ReservationTypeOnSite = "_reserved_by_onsite" // 现场预约
const ReservationTypeOnline = "_reserved_by_online" // 线上预约
const ReservationTypePhone = "_reserved_by_phone"   // 电话预约

const ReservationStatusDraft = "_draft"         // 状态草稿
const ReservationStatusConfirmed = "_confirmed" // 预约状态成功
const ReservationStatusCancelled = "_cancelled" // 预约状态取消
const ReservationStatusFailed = "_failed"       // 预约状态失败

type Reservation struct {
	powermodel.PowerModel

	ScheduleId          int64     `gorm:"comment:课程表Id" json:"scheduleId"`
	CustomerId          int64     `gorm:"comment:客户Id"  json:"customerId"`
	SourceChannelId     int64     `gorm:"comment:来源渠道Id"  json:"sourceChannelId"`
	ReservedArtisanId   int64     `gorm:"comment:预约的设计师Id"  json:"reservedArtisanId"`
	Type                int       `gorm:"comment:类型，包括在线，线下，电话等"  json:"type"`
	ReservedTime        time.Time `gorm:"comment:已预约时间"  json:"reservedTime"`
	CancelTime          time.Time `gorm:"comment:取消时间"  json:"cancelTime"`
	CheckinTime         time.Time `gorm:"comment:签到时间"  json:"checkinTime"`
	Description         string    `gorm:"comment:描述"  json:"description"`
	ConsumedPoints      int       `gorm:"comment:消耗点数，非必填"  json:"consumedPoints"`
	ConsumeMembershipId int64     `gorm:"comment:抵扣会籍Id"  json:"consumeMembershipId"`
	OperationStatus     int       `gorm:"comment:操作状态"  json:"operationStatus"`
	ReservationStatus   int       `gorm:"comment:预约状态"  json:"reservationStatus"`
}

const ReservationUniqueId = powermodel.UniqueId
