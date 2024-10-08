package product

import (
	"PowerX/internal/model"
	"PowerX/internal/model/powermodel"
)

// Table Name
func (mdl *PivotProductToProductCategory) TableName() string {
	return model.TableNamePivotProductToProductCategory
}

// 数据表结构
type PivotProductToProductCategory struct {
	powermodel.PowerPivot

	ProductId         int64 `gorm:"column:product_id; not null;index:idx_product_id" json:"productId"`
	ProductCategoryId int64 `gorm:"column:product_category_id; not null;index:idx_product_category_id" json:"productCategoryId"`
}

const PivotProductToCategoryForeignKey = "product_id"
const PivotProductToCategoryJoinKey = "product_category_id"

func (mdl *PivotProductToProductCategory) GetForeignKey() string {
	return PivotProductToCategoryForeignKey
}
func (mdl *PivotProductToProductCategory) GetForeignValue() int64 {
	return mdl.ProductId
}

func (mdl *PivotProductToProductCategory) GetJoinKey() string {
	return PivotProductToCategoryJoinKey
}
func (mdl *PivotProductToProductCategory) GetJoinValue() int64 {
	return mdl.ProductCategoryId
}
