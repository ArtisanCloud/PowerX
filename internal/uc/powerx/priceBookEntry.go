package powerx

import (
	"PowerX/internal/types/models"
	"github.com/ArtisanCloud/PowerLibs/v3/database"
	"github.com/ArtisanCloud/PowerLibs/v3/object"
)

func NewPriceBookEntry(mapObject *object.Collection) *models.PriceBookEntry {
	if mapObject == nil {
		mapObject = object.NewCollection(&object.HashMap{})
	}

	strPriceBookUUID := mapObject.GetString("priceBookUUID", "")
	strProductUUID := mapObject.GetString("productUUID", "")
	if strPriceBookUUID == "" || strProductUUID == "" {
		return nil
	}

	priceBookUUID := object.NewNullString(strPriceBookUUID, true)
	productUUID := object.NewNullString(strProductUUID, true)

	mdl := &models.PriceBookEntry{
		PowerModel:    database.NewPowerModel(),
		PriceBookUUID: priceBookUUID,
		ProductUUID:   productUUID,
		UnitPrice:     mapObject.GetFloat64("unitPrice", 0.0),
	}
	mdl.UniqueID = mdl.GetComposedUniqueID()

	return mdl
}
