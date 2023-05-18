package product

import (
	"PowerX/internal/model/powermodel"
)

// Table Name
func (mdl *PivotSkuToSpecificOption) TableName() string {
	return TableNamePivotSkuToSpecificOption
}

// 数据表结构
type PivotSkuToSpecificOption struct {
	powermodel.PowerPivot

	ProductId        int64 `gorm:"comment:产品Id; column:product_id; not null;index:idx_product_id" json:"productId"`
	SkuId            int64 `gorm:"comment:SkuId; column:sku_id; not null;index:idx_sku_id" json:"SkuId"`
	SpecificId       int64 `gorm:"comment:规格Id; column:specific_id; not null;index:specific_id" json:"specificId"`
	SpecificOptionId int64 `gorm:"comment:规格项Id; column:specific_option_id; not null;index:specific_option_id" json:"specificOptionId"`
	IsActivated      bool  `gorm:"comment:是否被激活; column:is_activated;" json:"isActivated,optional"`
}

const TableNamePivotSkuToSpecificOption = "pivot_sku_to_specific_options"
