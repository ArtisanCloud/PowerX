package wx

import (
	"database/sql"
	databasePowerLib "github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/configs/database"
	"gorm.io/gorm"
)

// TableName overrides the table name used by WXTagGroup to `profiles`
func (mdl *WXTagGroup) TableName() string {
	return mdl.GetTableName(true)
}

type WXTagGroup struct {
	WXTags       []*WXTag      `gorm:"foreignKey:WXTagGroupID;references:GroupID" json:"tags"`
	WXDepartment *WXDepartment `gorm:"foreignKey:WXDepartmentID;references:ID" json:"wxDepartment"`

	WXDepartmentID *int    `gorm:"column:wx_department_id" json:"wxDepartmentID"`
	GroupID        string  `gorm:"column:group_id;index:,unique;" json:"groupID"`
	GroupName      *string `gorm:"column:group_name" json:"groupName"`
	CreateTime     *int    `gorm:"column:create_time" json:"createTime"`
	Order          *int    `gorm:"column:order" json:"order"`
	Deleted        *bool   `gorm:"column:deleted" json:"deleted"`
}

const TABLE_NAME_TAG_GROUP = "wx_tag_groups"
const WX_TAG_GROUP_UNIQUE_ID = "group_id"

func NewWXTagGroup(mapObject *object.Collection) *WXTagGroup {
	if mapObject == nil {
		mapObject = object.NewCollection(&object.HashMap{})
	}

	tagsInterface := mapObject.Get("tags", nil)
	wxTags := []*WXTag{}
	if tagsInterface != nil {
		wxTags = tagsInterface.([]*WXTag)
	}

	tagGroup := &WXTagGroup{
		GroupID:        mapObject.GetString("groupID", ""),
		GroupName:      mapObject.GetStringPointer("groupName", ""),
		CreateTime:     mapObject.GetIntPointer("createTime", -1),
		Order:          mapObject.GetIntPointer("order", -1),
		Deleted:        mapObject.GetBoolPointer("deleted", false),
		WXDepartmentID: mapObject.GetIntPointer("wxDepartmentID", -1),
		WXTags:         wxTags,
	}

	return tagGroup
}

func (mdl *WXTagGroup) GetTableName(needFull bool) string {
	tableName := TABLE_NAME_TAG_GROUP
	if needFull {
		tableName = databasePowerLib.GetTableFullName(database.G_DBConfig.Schemas["default"], database.G_DBConfig.BaseConfig.Prefix, tableName)
	}
	return tableName
}

/**
 *  Relationships
 */

/**
 * Scope Where Conditions
 */
func (mdl *WXTagGroup) WhereWXTagGroupName(uuidOrPhone string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("uuid=@value OR mobile=@value", sql.Named("value", uuidOrPhone))
	}
}

func (mdl *WXTagGroup) WhereIsActive(db *gorm.DB) func(db *gorm.DB) *gorm.DB {
	//return db.Where("status = ?", "active")
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("active = ?", true)
	}
}
