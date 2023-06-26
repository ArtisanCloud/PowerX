package product

import (
	"PowerX/internal/model/powermodel"
	"PowerX/pkg/securityx"
	"fmt"
	"github.com/ArtisanCloud/PowerLibs/v3/object"
	"gorm.io/gorm"
)

// PriceBookEntry 数据表结构
type PriceBookEntry struct {
	powermodel.PowerModel

	//belongsTo
	Product      *Product       `gorm:"foreignKey:ProductId;references:Id" json:"product"`
	SKU          *SKU           `gorm:"foreignKey:SkuId;references:Id" json:"sku"`
	PriceBook    *PriceBook     `gorm:"foreignKey:PriceBookId;references:Id" json:"priceBook"`
	PriceConfigs []*PriceConfig `gorm:"foreignKey:PriceBookEntryId;references:Id" json:"priceConfigs"`

	UniqueID    object.NullString `gorm:"index:idx_price_book_id;index:idx_product_id;index:idx_sku_id;column:index_unique_id;unique;not null"`
	PriceBookId int64             `gorm:"index:idx_price_book_id;column:price_book_id;not null;" json:"priceBookId"`
	ProductId   int64             `gorm:"index:idx_product_id;column:product_id;not null;" json:"productId"`
	SkuId       int64             `gorm:"index:idx_sku_id;column:sku_id; comment:产品SKUId" json:"skuId"`
	UnitPrice   float64           `gorm:"type:decimal(10,2); comment:单价" json:"unitPrice"`
	ListPrice   float64           `gorm:"type:decimal(10,2); comment:零售价" json:"listPrice"`
	IsActive    bool              `gorm:"comment:是否激活" json:"isActive"`
}

const TableNamePriceBookEntry = "price_book_entries"
const PriceBookEntryUniqueId = "index_unique_id"

func (mdl *PriceBookEntry) GetComposedUniqueID() object.NullString {
	if mdl.PriceBookId > 0 && mdl.ProductId > 0 {
		strUniqueID := fmt.Sprintf("%d-%d-%d-%d", mdl.PriceBookId, mdl.ProductId, mdl.SkuId, mdl.DeletedAt.Time.Unix())
		strUniqueID = securityx.HashStringData(strUniqueID)
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
