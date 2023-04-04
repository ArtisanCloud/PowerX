package powerx

import (
	"PowerX/internal/types/models"
	"github.com/ArtisanCloud/PowerLibs/v3/object"
	"gorm.io/gorm"
)

type PriceBookUseCase struct {
	db *gorm.DB
}

func newPriceBookUseCase(db *gorm.DB) *PriceBookUseCase {
	return &PriceBookUseCase{
		db: db,
	}
}

func NewPriceBook(mapObject *object.Collection) *models.PriceBook {

	if mapObject == nil {
		mapObject = object.NewCollection(&object.HashMap{})
	}

	return &models.PriceBook{
		IsStandard: mapObject.GetBool("isStandard", false),
		Name:       mapObject.GetString("name", "untitled"),
		Region:     mapObject.GetInt8("region", 0),
		Level:      mapObject.GetInt8("level", 0),
		StoreUUID:  mapObject.GetString("storeUUID", ""),
	}
}
