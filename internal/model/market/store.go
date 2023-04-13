package reservationcenter

import (
	"PowerX/internal/model/powermodel"
)

type Store struct {
	powermodel.PowerModel

	//Manager *models.Employee `gorm:"comment:店长"`

	Name          string  `gorm:"comment:门店名称"`
	EmployeeID    int64   `gorm:"comment:店长ID"`
	ContactNumber string  `gorm:"comment:联系电话"`
	CoverURL      string  `gorm:"comment:封面图"`
	Address       string  `gorm:"comment:工作地址"`
	Longitude     float32 `gorm:"comment:经度"`
	Latitude      float32 `gorm:"comment:纬度"`
}

const StoreUniqueId = powermodel.UniqueId

const StoreLevelBasic = 1
const StoreLevelMedium = 2
const StoreLevelAdvanced = 3
