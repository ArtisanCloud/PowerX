package custom

import (
	"PowerX/internal/model/powermodel"
)

type ArtisanSpecific struct {
	powermodel.PowerModel

	ArtisanId int64 `gorm:"comment:ArtisanId"`
}

const ArtisanSpecificUniqueId = powermodel.UniqueId
