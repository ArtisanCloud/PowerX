package product

import (
	"PowerX/internal/model"
	"PowerX/internal/model/media"
	"PowerX/internal/model/powermodel"
	"gorm.io/gorm"
	"time"
)

type Artisan struct {
	powermodel.PowerModel

	// 如果要对元匠对象做特殊扩展开发，请在 /internal/model/custom/artisanspecific.go 中额外开发
	// 为了避免import Cycle，可以理解Artisan是一个标准的功能模块，基本上要扩展或者二开，是在外部对象去调用该标准对象，所以可以在custom里的model去引用标准对象
	//ArtisanSpecific *custom.ArtisanSpecific `gorm:"foreignKey:ArtisanId;references:Id" json:"specific"`

	PivotDetailImages    []*media.PivotMediaResourceToObject `gorm:"polymorphic:Object;polymorphicValue:artisans" json:"pivotDetailImages"`
	CoverImage           *media.MediaResource                `gorm:"foreignKey:CoverImageId;references:Id" json:"coverImage"`
	PivotStoreToArtisans []*PivotStoreToArtisan              `gorm:"foreignKey:ArtisanId;references:Id" json:"pivotStoreToArtisans"`

	UserId       int64     `gorm:"comment:员工Id"  json:"userId"`
	Name         string    `gorm:"comment:Artisan名称"  json:"name"`
	Level        int8      `gorm:"comment:级别"  json:"level"`
	Gender       bool      `gorm:"comment:性别"  json:"gender"`
	Birthday     time.Time `gorm:"comment:生日"  json:"birthday"`
	PhoneNumber  string    `gorm:"comment:手机号码"  json:"phoneNumber"`
	CoverImageId int64     `gorm:"comment:封面图Id" json:"coverImageId"`
	WorkNo       string    `gorm:"comment:工号" json:"workNo"`
	Email        string    `gorm:"comment:邮箱地址" json:"email"`
	Experience   string    `gorm:"comment:经验描述" json:"experience"`
	Specialty    string    `gorm:"comment:特长介绍" json:"specialty"`
	Certificate  string    `gorm:"comment:证书" json:"certificate"`
	Address      string    `gorm:"comment:工作地址" json:"address"`
}

const ArtisanUniqueId = powermodel.UniqueId

func (mdl *Artisan) TableName() string {
	return model.PowerXSchema + "." + model.TableNameArtisan
}

func (mdl *Artisan) GetTableName(needFull bool) string {
	tableName := model.TableNameArtisan
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
} // artisan level dd type
const ArtisanLevelType = "_artisan_level"

// artisan level dd items
const ArtisanLevelBasic = "_level_basic"
const ArtisanLevelMedium = "_level_medium"
const ArtisanLevelAdvanced = "_level_advanced"

func (mdl *Artisan) LoadPivotDetailImages(db *gorm.DB, conditions *map[string]interface{}) ([]*media.PivotMediaResourceToObject, error) {
	items := []*media.PivotMediaResourceToObject{}
	if conditions == nil {
		conditions = &map[string]interface{}{}
	}

	(*conditions)[media.PivotMediaResourceToObjectOwnerKey] = model.TableNameArtisan
	(*conditions)[media.PivotMediaResourceToObjectForeignKey] = mdl.Id

	err := powermodel.SelectMorphPivots(db, &media.PivotMediaResourceToObject{}, false, false, conditions).
		Preload("MediaResource").
		Find(&items).Error

	return items, err
}

func (mdl *Artisan) ClearPivotDetailImages(db *gorm.DB) error {
	conditions := &map[string]interface{}{}
	(*conditions)[media.PivotMediaResourceToObjectOwnerKey] = model.TableNameArtisan
	(*conditions)[media.PivotMediaResourceToObjectForeignKey] = mdl.Id
	return powermodel.ClearMorphPivots(db, &media.PivotMediaResourceToObject{}, false, false, conditions)
}

func (mdl *Artisan) ClearPivotStores(db *gorm.DB) error {
	conditions := &map[string]interface{}{}
	(*conditions)[PivotStoreToArtisanJoinKey] = mdl.Id
	return powermodel.ClearPivots(db, &PivotStoreToArtisan{}, false, false, conditions)
}
