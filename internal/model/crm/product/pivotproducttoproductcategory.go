package product

import (
	"PowerX/internal/model"
	"PowerX/internal/model/powermodel"
)

// 数据表结构
type PivotProductToProductCategory struct {
	powermodel.PowerPivot

	ProductId         int64 `gorm:"column:product_id; not null;index:idx_product_id" json:"productId"`
	ProductCategoryId int64 `gorm:"column:product_category_id; not null;index:idx_product_category_id" json:"productCategoryId"`
}

func (mdl *PivotProductToProductCategory) TableName() string {
	return model.PowerXSchema + "." + model.TableNamePivotProductToProductCategory
}

func (mdl *PivotProductToProductCategory) GetTableName(needFull bool) string {
	tableName := model.TableNamePivotProductToProductCategory
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
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
