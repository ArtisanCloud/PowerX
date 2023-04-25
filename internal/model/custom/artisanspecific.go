package custom

import (
	"PowerX/internal/model/powermodel"
)

type ArtisanSpecific struct {
	powermodel.PowerModel

	ArtisanId         int64 `gorm:"comment:ArtisanId"`
	Duration          int32 `gorm:"column:duration; comment:服务时长" json:"duration"`
	MandatoryDuration int32 `gorm:"column:mandatory_duration; comment:强制服务时长" json:"mandatoryDuration"`
}

const ArtisanSpecificUniqueId = powermodel.UniqueId
