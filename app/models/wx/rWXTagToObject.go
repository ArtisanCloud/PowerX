package wx

import (
	databasePowerLib "github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/configs/database"
	"gorm.io/gorm"
)

// TableName overrides the table name used by RWXTagToObject to `profiles`
func (mdl *RWXTagToObject) TableName() string {
	return mdl.GetTableName(true)
}

// r_tag_to_object 数据表结构
type RWXTagToObject struct {
	*databasePowerLib.PowerPivot

	//common fields
	UniqueID          object.NullString `gorm:"index:index_taggable_object_id;index:index_taggable_id;index;column:index_tag_to_object_id;unique"`
	TaggableOwnerType object.NullString `gorm:"column:taggable_owner_type;not null" json:"taggableOwnerType"`
	TaggableObjectID  object.NullString `gorm:"column:taggable_object_id;not null;index:index_taggable_object_id" json:"taggableObjectID"`
	TaggableID        object.NullString `gorm:"column:tag_id;not null;index:index_taggable_id" json:"taggableID"`
}

const TABLE_NAME_R_WX_TAG_TO_OBJECT = "r_wx_tag_to_object"

const R_WX_TAG_TO_OJECT_UNIQUE_ID = "index_wx_tag_to_object_id"

const R_WX_TAG_TO_OJECT_FOREIGN_KEY = "taggable_object_id"
const R_WX_TAG_TO_OJECT_OWNER_KEY = "taggable_owner_type"
const R_WX_TAG_TO_OJECT_JOIN_KEY = "tag_id"

func (mdl *RWXTagToObject) GetTableName(needFull bool) string {
	tableName := TABLE_NAME_R_WX_TAG_TO_OBJECT
	if needFull {
		tableName = databasePowerLib.GetTableFullName(database.G_DBConfig.Schemas["default"], database.G_DBConfig.BaseConfig.Prefix, tableName)
	}
	return tableName
}

func (mdl *RWXTagToObject) GetForeignKey() string {
	return R_WX_TAG_TO_OJECT_FOREIGN_KEY
}

func (mdl *RWXTagToObject) GetForeignValue() string {
	return mdl.TaggableObjectID.String
}

func (mdl *RWXTagToObject) GetJoinKey() string {
	return R_WX_TAG_TO_OJECT_JOIN_KEY
}

func (mdl *RWXTagToObject) GetJoinValue() string {
	return mdl.TaggableID.String
}

func (mdl *RWXTagToObject) GetOwnerKey() string {
	return R_WX_TAG_TO_OJECT_OWNER_KEY
}

func (mdl *RWXTagToObject) GetOwnerValue() string {
	return mdl.TaggableOwnerType.String
}

func (mdl *RWXTagToObject) GetPivotComposedUniqueID() string {
	return mdl.GetOwnerValue() + "-" + mdl.GetForeignValue() + "-" + mdl.GetJoinValue()
}

//--------------------------------------------------------------------

func (mdl *RWXTagToObject) GetPivots(db *gorm.DB) ([]*RWXTagToObject, error) {
	pivots := []*RWXTagToObject{}

	db = databasePowerLib.SelectMorphPivot(db, mdl)

	result := db.Find(&pivots)

	return pivots, result.Error

}

// --------------------------------------------------------------------
func (mdl *RWXTagToObject) MakePivotsFromObjectAndTags(obj databasePowerLib.ModelInterface, tags []*WXTag) ([]databasePowerLib.PivotInterface, error) {
	pivots := []databasePowerLib.PivotInterface{}
	for _, tag := range tags {
		pivot := &RWXTagToObject{
			TaggableOwnerType: object.NewNullString(obj.GetTableName(true), true),
			TaggableObjectID:  object.NewNullString(obj.GetForeignReferValue(), true),
			TaggableID:        object.NewNullString(tag.ID, true),
		}
		pivot.UniqueID = object.NewNullString(pivot.GetPivotComposedUniqueID(), true)

		pivots = append(pivots, pivot)
	}
	return pivots, nil
}
