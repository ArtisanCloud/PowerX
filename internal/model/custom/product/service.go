package product

import "PowerX/internal/model/powermodel"

type ServiceSpecific struct {
	powermodel.PowerModel

	ProductId         int64 `gorm:"column:product_id; comment:产品ID" json:"productId"`
	Duration          int32 `gorm:"column:duration; comment:服务时长" json:"duration"`
	MandatoryDuration int32 `gorm:"column:mandatory_duration; comment:强制服务时长" json:"mandatoryDuration"`
}

const ServiceSpecificUniqueId = powermodel.UniqueId
