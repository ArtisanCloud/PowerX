package product

import (
	"github.com/ArtisanCloud/PowerLibs/v3/database"
	"github.com/ArtisanCloud/PowerLibs/v3/object"
	"gorm.io/gorm"
)

// PriceBookEntry 数据表结构
type PriceBookEntry struct {
	*database.PowerModel

	//belongsTo
	Product      *Product       `gorm:"foreignKey:ProductUUID;references:UUID" json:"product"`
	PriceBook    *PriceBook     `gorm:"foreignKey:PriceBookUUID;references:UUID" json:"priceBook"`
	PriceConfigs []*PriceConfig `gorm:"foreignKey:PriceBookEntryUUID;references:UUID" json:"priceConfigs"`

	UniqueID      object.NullString `gorm:"index:index_PriceBookEntry_price_book_uuid;index:index_PriceBookEntry_product_uuid;index;column:index_price_book_entry_uuid;unique"`
	PriceBookUUID object.NullString `gorm:"index:index_PriceBookEntry_price_book_uuid;column:price_book_uuid;not null;" json:"priceBookUUID"`
	ProductUUID   object.NullString `gorm:"index:index_PriceBookEntry_product_uuid;column:product_uuid;not null;" json:"productUUID"`
	UnitPrice     float64           `gorm:"column:unit_price" json:"unitPrice"`
}

const TABLE_NAME_PRICE_BOOK_ENTRY = "price_book_entries"
const PRICE_BOOK_ENTRY_UNIQUE_ID = "index_price_book_entry_uuid"

func (mdl *PriceBookEntry) GetComposedUniqueID() object.NullString {
	if mdl.PriceBookUUID.String != "" && mdl.ProductUUID.String != "" {
		strUniqueID := mdl.PriceBookUUID.String + "-" + mdl.ProductUUID.String
		return object.NewNullString(strUniqueID, true)
	} else {
		return object.NewNullString("", false)
	}
}

func (mdl *PriceBookEntry) LoadProduct(db *gorm.DB, conditions *map[string]interface{}) (*Product, error) {
	product := &Product{}

	err := database.AssociationRelationship(db, conditions, mdl, "Product", true).Find(&product)
	mdl.Product = product

	return product, err
}
