package tag

import (
	"PowerX/internal/model"
	"PowerX/internal/model/powermodel"
	"PowerX/pkg/securityx"
	"fmt"
	"gorm.io/gorm"
)

// 数据表结构
type PivotObjectToTag struct {
	powermodel.PowerPivot

	Tag *Tag `gorm:"foreignKey:TagId,DataDictionaryKey;references:Id" json:"tag"`

	ObjectType string `gorm:"column:object_type; not null;index:idx_obj_type;comment:对象表名称" json:"objectOwner"`
	ObjectID   int64  `gorm:"column:object_id; not null;index:idx_object_id" json:"objectId"`
	TagId      int64  `gorm:"column:tag_id; not null;index:idx_tag_id" json:"tagId"`
}

const PivotObjectToTagForeignOwnerKey = "object_type"
const PivotObjectToTagForeignKey = "object_id"
const PivotObjectToTagJoinKey = "tag_id"

func (mdl *PivotObjectToTag) TableName() string {
	return model.PowerXSchema + "." + model.TableNamePivotObjectToTag
}

func (mdl *PivotObjectToTag) GetTableName(needFull bool) string {
	tableName := model.TableNamePivotObjectToTag
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}

func (mdl *PivotObjectToTag) GetOwnerKey() string {
	// 因为是morphy类型，所以外键是Owner
	return PivotObjectToTagForeignOwnerKey
}

func (mdl *PivotObjectToTag) GetOwnerValue() string {
	return mdl.ObjectType
}

func (mdl *PivotObjectToTag) GetForeignKey() string {
	return PivotObjectToTagForeignKey
}
func (mdl *PivotObjectToTag) GetForeignValue() int64 {
	return mdl.ObjectID
}

func (mdl *PivotObjectToTag) GetJoinKey() string {
	return PivotObjectToTagJoinKey
}
func (mdl *PivotObjectToTag) GetJoinValue() int64 {
	return mdl.TagId
}

func (mdl *PivotObjectToTag) GetPivotComposedUniqueID() string {
	key := fmt.Sprintf("%s-%s-%s-%s-%s-%s",
		mdl.GetOwnerKey(),
		mdl.GetOwnerValue(),
		mdl.GetForeignKey(),
		mdl.GetForeignValue(),
		mdl.GetJoinKey(),
		mdl.GetJoinKey(),
	)
	hashedId := securityx.HashStringData(key)

	return hashedId
}

//--------------------------------------------------------------------

func (mdl *PivotObjectToTag) GetMorphPivots(db *gorm.DB, where *map[string]interface{}) ([]*PivotObjectToTag, error) {
	pivots := []*PivotObjectToTag{}

	db = powermodel.SelectMorphPivot(db, mdl, where)

	result := db.Find(&pivots)

	return pivots, result.Error

}

// --------------------------------------------------------------------
func (mdl *PivotObjectToTag) MakeMorphPivotsFromObjectToDDs(obj powermodel.ModelInterface, tags []*Tag) ([]*PivotObjectToTag, error) {
	pivots := []*PivotObjectToTag{}
	for _, tag := range tags {
		pivot := &PivotObjectToTag{
			ObjectType: obj.GetTableName(false),
			ObjectID:   obj.GetForeignReferValue(),
			TagId:      tag.Id,
		}
		//pivot.UniqueID = pivot.GetPivotComposedUniqueID()

		pivots = append(pivots, pivot)
	}
	return pivots, nil
}

func GetItemIds(items []*PivotObjectToTag) []int64 {
	uniqueIds := make(map[int64]bool)
	arrayIds := []int64{}
	if len(items) <= 0 {
		return arrayIds
	}
	for _, item := range items {
		if item.TagId > 0 && !uniqueIds[item.Tag.Id] {
			arrayIds = append(arrayIds, item.Tag.Id)
			uniqueIds[item.TagId] = true
		}
	}
	return arrayIds
}
