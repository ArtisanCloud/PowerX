package product

import (
	"PowerX/internal/model/powermodel"
	"github.com/ArtisanCloud/PowerLibs/v3/database"
	"gorm.io/gorm"
	"time"
)

type Store struct {
	powermodel.PowerModel

	Artisans []*Artisan `gorm:"many2many:public.pivot_store_to_artisan;foreignKey:Id;joinForeignKey:StoreId;References:Id;JoinReferences:ArtisanId" json:"priceBooks"`

	EmployeeId    int64     `gorm:"comment:店长Id" json:"EmployeeId"`
	Name          string    `gorm:"comment:店铺名称" json:"name"`
	ContactNumber string    `gorm:"comment:手机号码" json:"contactNumber"`
	CoverURL      string    `gorm:"comment:封面图" json:"coverURL"`
	Email         string    `gorm:"comment:邮箱地址" json:"email"`
	Address       string    `gorm:"comment:工作地址" json:"address"`
	Description   string    `gorm:"comment:店铺描述" json:"description"`
	Longitude     float32   `gorm:"comment:经度" json:"longitude"`
	Latitude      float32   `gorm:"comment:纬度" json:"latitude"`
	StartWork     time.Time `gorm:"comment:开始工作时间" json:"startWork"`
	SndWork       time.Time `gorm:"comment:结束工作时间" json:"endWork"`
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
