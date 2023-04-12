package reservationcenter

import (
	"PowerX/internal/model/product"
)

type ServiceSpecific struct {
	product.ProductSpecific

	ProductId         int64 `gorm:"column:product_id; comment:产品ID" json:"productId"`
	Duration          int32 `gorm:"column:duration; comment:服务时长" json:"duration"`
	MandatoryDuration int32 `gorm:"column:mandatory_duration; comment:强制服务时长" json:"mandatoryDuration"`

	// 服务时长，单位分钟

}
