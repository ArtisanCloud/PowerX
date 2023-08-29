package media

import (
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types"
	"PowerX/pkg/securityx"
	"fmt"
	"gorm.io/gorm"
)

// Table Name
func (mdl *PivotMediaResourceToObject) TableName() string {
	return TableNamePivotMediaResourceToObject
}

// Pivot表
type PivotMediaResourceToObject struct {
	powermodel.PowerPivot

	MediaResource *MediaResource `gorm:"foreignKey:MediaResourceId;references:Id" json:"mediaResource"`

	// 所属键 owner key and value
	ObjectType string `gorm:"column:object_type; not null;index:idx_obj_type;comment:对象表名称" json:"objectOwner"`
	// 外键foreign key and value
	ObjectId int64 `gorm:"column:object_id; not null;index:idx_obj_id;comment:对象Id" json:"objectId"`
	// 引用键 join key and value
	MediaResourceId int64 `gorm:"column:media_id; not null;index:idx_media_id;comment:媒体资源Id" json:"dataDictionaryType"`

	Sort int `gorm:"comment:排序，越大约靠前"`

	MediaUsage string `gorm:"column:media_usage; comment:自定义用途" json:"mediaUsage"`
}

const TableNamePivotMediaResourceToObject = "pivot_media_resource_to_object"

const PivotMediaResourceToObjectOwnerKey = "object_type"
const PivotMediaResourceToObjectForeignKey = "object_id"
const PivotMediaResourceToObjectJoinKey = "media_id"

func (mdl *PivotMediaResourceToObject) GetOwnerKey() string {
	// 因为是morphy类型，所以外键是Owner
	return PivotMediaResourceToObjectOwnerKey
}
func (mdl *PivotMediaResourceToObject) GetOwnerValue() string {
	return mdl.ObjectType
}

func (mdl *PivotMediaResourceToObject) GetForeignKey() string {
	return PivotMediaResourceToObjectForeignKey
}
func (mdl *PivotMediaResourceToObject) GetForeignValue() int64 {
	return mdl.ObjectId
}

func (mdl *PivotMediaResourceToObject) GetJoinKey() string {
	return PivotMediaResourceToObjectJoinKey
}
func (mdl *PivotMediaResourceToObject) GetJoinValue() int64 {
	return mdl.MediaResourceId
}

func (mdl *PivotMediaResourceToObject) GetPivotComposedUniqueID() string {
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

func (mdl *PivotMediaResourceToObject) GetMorphPivots(db *gorm.DB, where *map[string]interface{}) ([]*PivotMediaResourceToObject, error) {
	pivots := []*PivotMediaResourceToObject{}

	db = powermodel.SelectMorphPivot(db, mdl, where)

	result := db.Find(&pivots)

	return pivots, result.Error

}

// --------------------------------------------------------------------
func (mdl *PivotMediaResourceToObject) MakeMorphPivotsFromObjectToMediaResources(obj powermodel.ModelInterface, mediaResources []*MediaResource, mediaUsage string) ([]*PivotMediaResourceToObject, error) {
	pivots := []*PivotMediaResourceToObject{}
	for _, mediaResource := range mediaResources {
		pivot := &PivotMediaResourceToObject{
			ObjectType:      obj.GetTableName(false),
			ObjectId:        obj.GetForeignReferValue(),
			MediaResourceId: mediaResource.Id,
			MediaUsage:      mediaUsage,
		}
		//pivot.UniqueID = pivot.GetPivotComposedUniqueID()

		pivots = append(pivots, pivot)
	}
	return pivots, nil
}

func FindSortIndexById(items []*types.SortIdItem, targetID int64) int {
	for _, item := range items {
		if item.Id == targetID {
			return item.SortIndex
		}
	}
	return -1 // 如果没有找到匹配的ID，则返回-1表示未找到
}
