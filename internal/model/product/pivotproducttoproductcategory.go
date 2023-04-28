package product

import (
	"PowerX/internal/model/powermodel"
)

// Table Name
func (mdl *PivotProductToProductCategory) TableName() string {
	return TableNamePivotProductToProductCategory
}

// 数据表结构
type PivotProductToProductCategory struct {
	powermodel.PowerPivot

	ProductId         int64 `gorm:"column:product_id; not null;index:idx_product_id" json:"productId"`
	ProductCategoryId int64 `gorm:"column:product_category_id; not null;index:idx_product_category_id" json:"productCategoryId"`
}

const TableNamePivotProductToProductCategory = "pivot_product_to_product_category"
