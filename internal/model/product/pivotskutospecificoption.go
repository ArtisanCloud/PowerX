package product

import (
	"PowerX/internal/model/powermodel"
	"PowerX/pkg/securityx"
	"fmt"
	"github.com/ArtisanCloud/PowerLibs/v3/object"
)

// Table Name
func (mdl *PivotSkuToSpecificOption) TableName() string {
	return TableNamePivotSkuToSpecificOption
}

// 数据表结构
type PivotSkuToSpecificOption struct {
	powermodel.PowerPivot

	UniqueID         object.NullString `gorm:"index:idx_unique_id;index:idx_product_id;index:idx_sku_id;index:idx_sku_id;index:index_specific_id;index:index_specific_option_id;column:index_unique_id;unique;not null"`
	ProductId        int64             `gorm:"comment:产品Id; column:product_id; not null;index:idx_product_id" json:"productId"`
	SkuId            int64             `gorm:"comment:SkuId; column:sku_id; not null;index:idx_sku_id" json:"SkuId"`
	SpecificId       int64             `gorm:"comment:规格Id; column:specific_id; not null;index:index_specific_id" json:"specificId"`
	SpecificOptionId int64             `gorm:"comment:规格项Id; column:specific_option_id; not null;index:index_specific_option_id" json:"specificOptionId"`
	IsActivated      bool              `gorm:"comment:是否被激活; column:is_activated;" json:"isActivated,optional"`
}

const TableNamePivotSkuToSpecificOption = "pivot_sku_to_specific_options"

func (mdl *PivotSkuToSpecificOption) GetPivotComposedUniqueID() object.NullString {
	if mdl.ProductId > 0 && mdl.SkuId > 0 && mdl.SpecificId > 0 && mdl.SpecificOptionId > 0 {
		strUniqueID := fmt.Sprintf("%d-%d-%d-%d", mdl.ProductId, mdl.SkuId, mdl.SpecificId, mdl.SpecificOptionId)
		strUniqueID = securityx.HashStringData(strUniqueID)
		return object.NewNullString(strUniqueID, true)
	} else {
		return object.NewNullString("", false)
	}
}
