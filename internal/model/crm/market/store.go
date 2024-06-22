package market

import (
	"PowerX/internal/model/crm/product"
	"PowerX/internal/model/media"
	"PowerX/internal/model/powermodel"
	"gorm.io/gorm"
	"time"
)

type Store struct {
	powermodel.PowerModel

	Artisans          []*product.Artisan                  `gorm:"many2many:public.pivot_store_to_artisan;foreignKey:Id;joinForeignKey:StoreId;References:Id;JoinReferences:ArtisanId" json:"priceBooks"`
	PivotDetailImages []*media.PivotMediaResourceToObject `gorm:"polymorphic:Object;polymorphicValue:stores" json:"pivotDetailImages"`
	CoverImage        *media.MediaResource                `gorm:"foreignKey:CoverImageId;references:Id" json:"coverImage"`

	StoreUserId   int64     `gorm:"comment:店长Id" json:"storeUserId"`
	Name          string    `gorm:"comment:店铺名称" json:"name"`
	ContactNumber string    `gorm:"comment:手机号码" json:"contactNumber"`
	CoverImageId  int64     `gorm:"comment:封面图Id" json:"coverImageId"`
	Email         string    `gorm:"comment:邮箱地址" json:"email"`
	Address       string    `gorm:"comment:工作地址" json:"address"`
	Description   string    `gorm:"comment:店铺描述" json:"description"`
	Longitude     float32   `gorm:"comment:经度" json:"longitude"`
	Latitude      float32   `gorm:"comment:纬度" json:"latitude"`
	StartWork     time.Time `gorm:"comment:开始工作时间" json:"startWork"`
	EndWork       time.Time `gorm:"comment:结束工作时间" json:"endWork"`
}

const TableNameStore = "stores"
const StoreUniqueId = powermodel.UniqueId

func (mdl *Store) LoadArtisans(db *gorm.DB, conditions *map[string]interface{}, withClauseAssociations bool) error {

	mdl.Artisans = []*product.Artisan{}
	err := powermodel.AssociationRelationship(db, conditions, mdl, "Artisans", false).Find(&mdl.Artisans)
	//fmt.Dump(mdl.Artisans)
	return err
}

func (mdl *Store) ClearArtisans(db *gorm.DB) error {

	var err error
	// 清除元匠的关联
	err = db.Model(mdl).Association("Artisans").Clear()

	return err
}

func (mdl *Store) LoadPivotDetailImages(db *gorm.DB, conditions *map[string]interface{}) ([]*media.PivotMediaResourceToObject, error) {
	items := []*media.PivotMediaResourceToObject{}
	if conditions == nil {
		conditions = &map[string]interface{}{}
	}

	(*conditions)[media.PivotMediaResourceToObjectOwnerKey] = TableNameStore
	(*conditions)[media.PivotMediaResourceToObjectForeignKey] = mdl.Id

	err := powermodel.SelectMorphPivots(db, &media.PivotMediaResourceToObject{}, false, false, conditions).
		Preload("MediaResource").
		Find(&items).Error

	return items, err
}

func (mdl *Store) ClearPivotDetailImages(db *gorm.DB) error {
	conditions := &map[string]interface{}{}
	(*conditions)[media.PivotMediaResourceToObjectOwnerKey] = TableNameStore
	(*conditions)[media.PivotMediaResourceToObjectForeignKey] = mdl.Id
	return powermodel.ClearMorphPivots(db, &media.PivotMediaResourceToObject{}, false, false, conditions)
}

func MakePivotsFromArtisansToStores(artisans []*product.Artisan, stores []*Store) []*product.PivotStoreToArtisan {
	pivots := []*product.PivotStoreToArtisan{}
	for _, artisan := range artisans {
		for _, store := range stores {
			pivot := &product.PivotStoreToArtisan{
				ArtisanId: artisan.Id,
				StoreId:   store.Id,
			}
			pivots = append(pivots, pivot)
		}
	}
	return pivots
}

func MakePivotsFromArtisanIdsToStoreIds(artisanIds []int64, storeIds []int64) []*product.PivotStoreToArtisan {
	pivots := []*product.PivotStoreToArtisan{}
	for _, artisanId := range artisanIds {
		for _, storeId := range storeIds {
			pivot := &product.PivotStoreToArtisan{
				ArtisanId: artisanId,
				StoreId:   storeId,
			}
			pivots = append(pivots, pivot)
		}
	}
	return pivots
}
