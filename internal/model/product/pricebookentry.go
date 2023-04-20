package product

import (
	"PowerX/internal/model/powermodel"
	"fmt"
	"github.com/ArtisanCloud/PowerLibs/v3/object"
	"gorm.io/gorm"
)

// PriceBookEntry 数据表结构
type PriceBookEntry struct {
	powermodel.PowerModel

	//belongsTo
	Product      *Product       `gorm:"foreignKey:ProductId;references:Id" json:"product"`
	PriceBook    *PriceBook     `gorm:"foreignKey:PriceBookId;references:Id" json:"priceBook"`
	PriceConfigs []*PriceConfig `gorm:"foreignKey:PriceBookEntryId;references:Id" json:"priceConfigs"`

	UniqueID    object.NullString `gorm:"index:idx_price_book_entry_price_book_id;index:idx_price_book_entry_product_id;index;column:idx_price_book_entry_id;unique"`
	PriceBookId int64             `gorm:"index:idx_price_book_entry_price_book_id;column:price_book_id;not null;" json:"priceBookId"`
	ProductId   int64             `gorm:"index:idx_price_book_entry_product_id;column:product_id;not null;" json:"productId"`
	UnitPrice   float64           `gorm:"column:unit_price; comment:单价" json:"unitPrice"`
	RetailPrice float64           `gorm:"column:retail_price; comment:零售价" json:"retailPrice"`
	IsActive    bool              `gorm:"column:is_active; comment:是否激活" json:"isActive"`
	ProductSpecific
}

const TableNamePriceBookEntry = "price_book_entries"
const PriceBookEntryUniqueId = "idx_price_book_entry_id"

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
