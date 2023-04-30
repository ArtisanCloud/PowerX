package product

import (
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types"
	"time"
)

type Artisan struct {
	powermodel.PowerModel

	// 如果要对元匠对象做特殊扩展开发，请在 /internal/model/custom/artisanspecific.go 中额外开发
	// 为了避免import Cycle，可以理解Artisan是一个标准的功能模块，基本上要扩展或者二开，是在外部对象去调用该标准对象，所以可以在custom里的model去引用标准对象
	//ArtisanSpecific *custom.ArtisanSpecific `gorm:"foreignKey:ArtisanId;references:Id" json:"specific"`

	EmployeeId  int64     `gorm:"comment:员工Id"`
	Name        string    `gorm:"comment:Artisan名称"`
	Level       int8      `gorm:"comment:级别"`
	Gender      string    `gorm:"comment:性别"`
	Birthday    time.Time `gorm:"comment:生日"`
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

// artisan level dd type
const ArtisanLevelType = "_artisan_level"

// artisan level dd items
const ArtisanLevelBasic = "_level_basic"
const ArtisanLevelMedium = "_level_medium"
const ArtisanLevelAdvanced = "_level_advanced"

type FindArtisanOption struct {
	OrderBy string
	Ids     []int64
	Names   []string
	types.PageEmbedOption
}
