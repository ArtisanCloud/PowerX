package product

import (
	"PowerX/internal/model/powermodel"
	"fmt"
	"github.com/ArtisanCloud/PowerLibs/v3/object"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// PriceBookEntry 数据表结构
type PriceBookEntry struct {
	powermodel.PowerModel

	//belongsTo
	Product      *Product       `gorm:"foreignKey:ProductId;references:Id" json:"product"`
	PriceBook    *PriceBook     `gorm:"foreignKey:PriceBookId;references:Id" json:"priceBook"`
	PriceConfigs []*PriceConfig `gorm:"foreignKey:PriceBookEntryId;references:Id" json:"priceConfigs"`

	UniqueID    object.NullString `gorm:"index:index_price_book_entry_price_book_id;index:index_price_book_entry_product_id;index;column:index_price_book_entry_id;unique"`
	PriceBookId int64             `gorm:"index:index_price_book_entry_price_book_id;column:price_book_id;not null;" json:"priceBookId"`
	ProductId   int64             `gorm:"index:index_price_book_entry_product_id;column:product_id;not null;" json:"productId"`
	UnitPrice   float64           `gorm:"column:unit_price" json:"unitPrice"`
	RetailPrice float64           `gorm:"column:retail_price" json:"retailPrice"`
	Inventory   int16             `gorm:"column:inventory" json:"inventory"`
	Weight      float32           `gorm:"column:weight" json:"weight"`
	Volume      float32           `gorm:"column:volume" json:"volume"`
	Encode      string            `gorm:"column:encode" json:"encode"`
	BarCode     string            `gorm:"column:bar_code" json:"barCode"`
	Extra       datatypes.JSON    `gorm:"column:extra" json:"extra"`
}

const TableNamePriceBookEntry = "price_book_entries"
const PriceBookEntryUniqueId = "index_price_book_entry_id"

func (mdl *PriceBookEntry) GetComposedUniqueID() object.NullString {
	if mdl.PriceBookId > 0 && mdl.ProductId > 0 {
		strUniqueID := fmt.Sprintf("%d-%d", mdl.PriceBookId, mdl.ProductId)
		return object.NewNullString(strUniqueID, true)
	} else {
		return object.NewNullString("", false)
	}
}

func (mdl *PriceBookEntry) LoadProduct(db *gorm.DB, conditions *map[string]interface{}) (*Product, error) {
	product := &Product{}

	err := powermodel.AssociationRelationship(db, conditions, mdl, "Product", true).Find(&product)
	mdl.Product = product

	return product, err
}
