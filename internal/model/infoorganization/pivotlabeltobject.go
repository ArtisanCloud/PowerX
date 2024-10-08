package infoorganizatoin

import (
	"PowerX/internal/model"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types"
	"PowerX/pkg/securityx"
	"fmt"
	"gorm.io/gorm"
)

// Table Name
func (mdl *PivotLabelToObject) TableName() string {
	return model.TableNamePivotLabelToObject
}

// Pivot表
type PivotLabelToObject struct {
	powermodel.PowerPivot

	// 所属键 owner key and value
	ObjectType string `gorm:"column:object_type; not null;index:idx_obj_type;comment:对象表名称" json:"objectOwner"`
	// 外键foreign key and value
	ObjectID int64 `gorm:"column:object_id; not null;index:idx_obj_id;comment:对象Id" json:"objectId"`
	// 引用键 join key and value
	LabelId int64 `gorm:"column:label_id; not null;index:idx_label_id;comment:类别Id" json:"labelId"`

	Sort int `gorm:"comment:排序，越大约靠前"`
}

const PivotLabelToObjectOwnerKey = "object_type"
const PivotLabelToObjectForeignKey = "object_id"
const PivotLabelToObjectJoinKey = "label_id"

func (mdl *PivotLabelToObject) GetOwnerKey() string {
	// 因为是morphy类型，所以外键是Owner
	return PivotLabelToObjectOwnerKey
}
func (mdl *PivotLabelToObject) GetOwnerValue() string {
	return mdl.ObjectType
}

func (mdl *PivotLabelToObject) GetForeignKey() string {
	return PivotLabelToObjectForeignKey
}
func (mdl *PivotLabelToObject) GetForeignValue() int64 {
	return mdl.ObjectID
}

func (mdl *PivotLabelToObject) GetJoinKey() string {
	return PivotLabelToObjectJoinKey
}
func (mdl *PivotLabelToObject) GetJoinValue() int64 {
	return mdl.LabelId
}

func (mdl *PivotLabelToObject) GetPivotComposedUniqueID() string {
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

func (mdl *PivotLabelToObject) GetMorphPivots(db *gorm.DB, where *map[string]interface{}) ([]*PivotLabelToObject, error) {
	pivots := []*PivotLabelToObject{}

	db = powermodel.SelectMorphPivot(db, mdl, where)

	result := db.Find(&pivots)

	return pivots, result.Error

}

// --------------------------------------------------------------------
func (mdl *PivotLabelToObject) MakeMorphPivotsFromObjectToLabels(obj powermodel.ModelInterface, labels []*Label) ([]*PivotLabelToObject, error) {
	pivots := []*PivotLabelToObject{}
	for _, label := range labels {
		pivot := &PivotLabelToObject{
			ObjectType: obj.GetTableName(false),
			ObjectID:   obj.GetForeignReferValue(),
			LabelId:    label.Id,
		}
		//pivot.UniqueID = pivot.GetPivotComposedUniqueID()

		pivots = append(pivots, pivot)
	}
	return pivots, nil
}

func (mdl *PivotLabelToObject) FindSortIndexById(items []*types.SortIdItem, targetID int64) int {
	for _, item := range items {
		if item.Id == targetID {
			return item.SortIndex
		}
	}
	return -1 // 如果没有找到匹配的ID，则返回-1表示未找到
}
