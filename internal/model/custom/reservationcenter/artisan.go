package reservationcenter

import (
	"PowerX/internal/model/powermodel"
	"time"
)

type Artisan struct {
	powermodel.PowerModel

	Name        string    `gorm:"comment:设计师名称"`
	Level       int8      `gorm:"comment:级别"`
	Gender      string    `gorm:"comment:性别"`
	birthday    time.Time `gorm:"comment:生日"`
	PhoneNumber string    `gorm:"comment:手机号码"`
	CoverURL    string    `gorm:"comment:封面图"`
	WorkNo      string    `gorm:"comment:工号"`
	Email       string    `gorm:"comment:邮箱地址"`
	Experience  uint      `gorm:"comment:经验描述"`
	Specialty   string    `gorm:"comment:特长介绍"`
	Certificate string    `gorm:"comment:证书"`
	Address     string    `gorm:"comment:工作地址"`
}

const ArtisanUniqueId = powermodel.UniqueId

const ArtisanLevelBasic = 1
const ArtisanLevelMedium = 2
const ArtisanLevelAdvanced = 3
