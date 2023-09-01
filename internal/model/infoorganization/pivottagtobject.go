package infoorganizatoin

import (
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types"
	"PowerX/pkg/securityx"
	"fmt"
	"gorm.io/gorm"
)

// Table Name
func (mdl *PivotTagToObject) TableName() string {
	return TableNamePivotTagToObject
}

// Pivot表
type PivotTagToObject struct {
	powermodel.PowerPivot

	// 所属键 owner key and value
	ObjectType string `gorm:"column:object_type; not null;index:idx_obj_type;comment:对象表名称" json:"objectOwner"`
	// 外键foreign key and value
	ObjectID int64 `gorm:"column:object_id; not null;index:idx_obj_id;comment:对象Id" json:"objectId"`
	// 引用键 join key and value
	TagId int64 `gorm:"column:tag_id; not null;index:idx_tag_id;comment:类别Id" json:"tagId"`

	Sort int `gorm:"comment:排序，越大约靠前"`
}

const TableNamePivotTagToObject = "pivot_tag_to_object"

const PivotTagToObjectOwnerKey = "object_type"
const PivotTagToObjectForeignKey = "object_id"
const PivotTagToObjectJoinKey = "tag_id"

func (mdl *PivotTagToObject) GetOwnerKey() string {
	// 因为是morphy类型，所以外键是Owner
	return PivotTagToObjectOwnerKey
}
func (mdl *PivotTagToObject) GetOwnerValue() string {
	return mdl.ObjectType
}

func (mdl *PivotTagToObject) GetForeignKey() string {
	return PivotTagToObjectForeignKey
}
func (mdl *PivotTagToObject) GetForeignValue() int64 {
	return mdl.ObjectID
}

func (mdl *PivotTagToObject) GetJoinKey() string {
	return PivotTagToObjectJoinKey
}
func (mdl *PivotTagToObject) GetJoinValue() int64 {
	return mdl.TagId
}

func (mdl *PivotTagToObject) GetPivotComposedUniqueID() string {
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

func (mdl *PivotTagToObject) GetMorphPivots(db *gorm.DB, where *map[string]interface{}) ([]*PivotTagToObject, error) {
	pivots := []*PivotTagToObject{}

	db = powermodel.SelectMorphPivot(db, mdl, where)

	result := db.Find(&pivots)

	return pivots, result.Error

}

// --------------------------------------------------------------------
func (mdl *PivotTagToObject) MakeMorphPivotsFromObjectToTags(obj powermodel.ModelInterface, tags []*Tag) ([]*PivotTagToObject, error) {
	pivots := []*PivotTagToObject{}
	for _, tag := range tags {
		pivot := &PivotTagToObject{
			ObjectType: obj.GetTableName(false),
			ObjectID:   obj.GetForeignReferValue(),
			TagId:      tag.Id,
		}
		//pivot.UniqueID = pivot.GetPivotComposedUniqueID()

		pivots = append(pivots, pivot)
	}
	return pivots, nil
}

func (mdl *PivotTagToObject) FindSortIndexById(items []*types.SortIdItem, targetID int64) int {
	for _, item := range items {
		if item.Id == targetID {
			return item.SortIndex
		}
	}
	return -1 // 如果没有找到匹配的ID，则返回-1表示未找到
}
