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

	ProductID         int64 `gorm:"column:product_id; not null;index:index_product_id" json:"productID"`
	ProductCategoryID int64 `gorm:"column:wechat_mp_product_id; not null;index:index_product_category_id" json:"productCategoryID"`
}

const TableNamePivotProductToProductCategory = "pivot_product_to_product_category"
