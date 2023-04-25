package product

import (
	"PowerX/internal/model/powermodel"
	"github.com/ArtisanCloud/PowerLibs/v3/database"
	"gorm.io/gorm"
)

type Store struct {
	powermodel.PowerModel

	Artisans []*Artisan `gorm:"many2many:public.pivot_store_to_artisan;foreignKey:Id;joinForeignKey:StoreId;References:Id;JoinReferences:ArtisanId" json:"priceBooks"`

	Name        string `gorm:"comment:店铺名称"`
	PhoneNumber string `gorm:"comment:手机号码"`
	CoverURL    string `gorm:"comment:封面图"`
	Email       string `gorm:"comment:邮箱地址"`
	Address     string `gorm:"comment:工作地址"`
	Description string `gorm:"comment:店铺描述"`
}

const StoreUniqueId = powermodel.UniqueId

func (mdl *Store) LoadArtisans(db *gorm.DB, conditions *map[string]interface{}, withClauseAssociations bool) error {

	mdl.Artisans = []*Artisan{}
	err := database.AssociationRelationship(db, conditions, mdl, "Artisans", false).Find(&mdl.Artisans)
	//fmt.Dump(mdl.Artisans)
	return err
}

func (mdl *Store) ClearArtisans(db *gorm.DB) error {

	var err error
	// 清除元匠的关联
	err = db.Model(mdl).Association("Artisans").Clear()

	return err
}
