package infoorganizatoin

import (
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types"
	"PowerX/pkg/securityx"
	"fmt"
	"gorm.io/gorm"
)

// Table Name
func (mdl *PivotCategoryToObject) TableName() string {
	return TableNamePivotCategoryToObject
}

// Pivot表
type PivotCategoryToObject struct {
	powermodel.PowerPivot

	// 所属键 owner key and value
	ObjectType string `gorm:"column:object_type; not null;index:idx_obj_type;comment:对象表名称" json:"objectOwner"`
	// 外键foreign key and value
	ObjectID int64 `gorm:"column:object_id; not null;index:idx_obj_id;comment:对象Id" json:"objectId"`
	// 引用键 join key and value
	CategoryId int64 `gorm:"column:category_id; not null;index:idx_category_id;comment:类别Id" json:"categoryId"`

	Sort int `gorm:"comment:排序，越大约靠前"`
}

const TableNamePivotCategoryToObject = "pivot_category_to_object"

const PivotCategoryToObjectOwnerKey = "object_type"
const PivotCategoryToObjectForeignKey = "object_id"
const PivotCategoryToObjectJoinKey = "category_id"

func (mdl *PivotCategoryToObject) GetOwnerKey() string {
	// 因为是morphy类型，所以外键是Owner
	return PivotCategoryToObjectOwnerKey
}
func (mdl *PivotCategoryToObject) GetOwnerValue() string {
	return mdl.ObjectType
}

func (mdl *PivotCategoryToObject) GetForeignKey() string {
	return PivotCategoryToObjectForeignKey
}
func (mdl *PivotCategoryToObject) GetForeignValue() int64 {
	return mdl.ObjectID
}

func (mdl *PivotCategoryToObject) GetJoinKey() string {
	return PivotCategoryToObjectJoinKey
}
func (mdl *PivotCategoryToObject) GetJoinValue() int64 {
	return mdl.CategoryId
}

func (mdl *PivotCategoryToObject) GetPivotComposedUniqueID() string {
	key := fmt.Sprintf("%s-%s-%d-%d",
		mdl.GetOwnerKey(),
		mdl.GetOwnerValue(),
		mdl.GetForeignValue(),
		mdl.GetJoinValue(),
	)
	hashedId := securityx.HashStringData(key)

	return hashedId
}

//--------------------------------------------------------------------

func (mdl *PivotCategoryToObject) GetMorphPivots(db *gorm.DB, where *map[string]interface{}) ([]*PivotCategoryToObject, error) {
	pivots := []*PivotCategoryToObject{}

	db = powermodel.SelectMorphPivot(db, mdl, where)

	result := db.Find(&pivots)

	return pivots, result.Error

}

// --------------------------------------------------------------------
func (mdl *PivotCategoryToObject) MakeMorphPivotsFromObjectToCategorys(obj powermodel.ModelInterface, categories []*Category) ([]*PivotCategoryToObject, error) {
	pivots := []*PivotCategoryToObject{}
	for _, category := range categories {
		pivot := &PivotCategoryToObject{
			ObjectType: obj.GetTableName(false),
			ObjectID:   obj.GetForeignReferValue(),
			CategoryId: category.Id,
		}
		//pivot.UniqueID = pivot.GetPivotComposedUniqueID()

		pivots = append(pivots, pivot)
	}
	return pivots, nil
}

func (mdl *PivotCategoryToObject) FindSortIndexById(items []*types.SortIdItem, targetID int64) int {
	for _, item := range items {
		if item.Id == targetID {
			return item.SortIndex
		}
	}
	return -1 // 如果没有找到匹配的ID，则返回-1表示未找到
}
