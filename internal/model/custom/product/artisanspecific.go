package product

import (
	"PowerX/internal/model/powermodel"
)

type ArtisanSpecific struct {
	powermodel.PowerModel

	ArtisanId          int64 `gorm:"comment:ArtisanId; comment:元匠Id" json:"artisanId"`
	MaxServiceDuration int32 `gorm:"column:duration; comment:服务时长" json:"maxServiceDuration"`
	MandatoryDuration  int32 `gorm:"column:mandatory_duration; comment:强制服务时长" json:"mandatoryDuration"`
}

const ArtisanSpecificUniqueId = powermodel.UniqueId
