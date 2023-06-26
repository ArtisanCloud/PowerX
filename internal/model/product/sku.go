package product

import (
	"PowerX/internal/model/powermodel"
	"PowerX/pkg/securityx"
	"fmt"
	"github.com/ArtisanCloud/PowerLibs/v3/object"
	"gorm.io/datatypes"
)

// SKU 数据表结构
type SKU struct {
	powermodel.PowerModel

	PivotSkuToSpecificOptions []*PivotSkuToSpecificOption `gorm:"foreignKey:SkuId;references:Id" json:"pivotSkuToSpecificOptions"`
	PriceBookEntry            *PriceBookEntry             `gorm:"foreignKey:SkuId;references:Id" json:"priceBookEntry"`

	UniqueID  object.NullString `gorm:"index:idx_unique_id;index:idx_product_id;column:index_unique_id;unique;not null"`
	ProductId int64             `gorm:"index:idx_product_id;not null;" json:"productId"`
	SkuNo     string            `gorm:"comment:SKU编号" json:"sku"`
	Inventory int               `gorm:"comment:库存数量" json:"inventory"`
	OptionIds datatypes.JSON    `gorm:"comment:规格Ids" json:"OptionIds"`
}

const TableNameSKU = "sku"
const SkuUniqueId = "index_unique_id"

func (mdl *SKU) GetComposedUniqueID() object.NullString {
	if len(mdl.OptionIds) > 0 && mdl.ProductId > 0 {
		strUniqueID := fmt.Sprintf("%d-%s-%d", mdl.ProductId, mdl.OptionIds.String(), mdl.DeletedAt.Time.Unix())
		strUniqueID = securityx.HashStringData(strUniqueID)
		return object.NewNullString(strUniqueID, true)
	} else {
		return object.NewNullString("", false)
	}
}
