package reservationcenter

import (
	"PowerX/internal/model/powermodel"
)

type CheckinLog struct {
	powermodel.PowerModel

	ConsumedPoints int8   `gorm:"comment:消耗点数"`
	MembershipId   int64  `gorm:"comment:使用会籍Id"`
	Name           string `gorm:"comment:客户名称"`
	ReservationId  int64  `gorm:"comment:预约Id"`
}

const CheckinLogUniqueId = powermodel.UniqueId

const CheckinLogLevelBasic = 1
const CheckinLogLevelMedium = 2
const CheckinLogLevelAdvanced = 3
