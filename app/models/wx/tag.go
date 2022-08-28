package wx

import (
	"database/sql"
	databasePowerLib "github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TableName overrides the table name used by WXTag to `profiles`
func (mdl *WXTag) TableName() string {
	return mdl.GetTableName(true)
}

type WXTag struct {
	TagGroup *WXTagGroup `gorm:"foreignKey:WXTagGroupID;references:GroupID" json:"tagGroup"`

	WXDepartmentID *int    `gorm:"column:wx_department_id" json:"wxDepartmentID"`
	TempID         *string `gorm:"column:temp_id" json:"temp_id"`
	ID             string  `gorm:"column:wx_id;index:,unique;" json:"wx_id"`
	Name           *string `gorm:"column:name" json:"name"`
	CreateTime     *int    `gorm:"column:create_time" json:"createTime"`
	Order          *int    `gorm:"column:order" json:"order"`
	Deleted        *bool   `gorm:"column:deleted" json:"deleted"`
	WXTagGroupID   *string `gorm:"column:wx_tag_group_id" json:"wxTagGroupID"`
}

const TABLE_NAME_TAG = "wx_tags"
const WX_TAG_UNIQUE_ID = "wx_id"

func NewWXTag(mapObject *object.Collection) *WXTag {
	if mapObject == nil {
		mapObject = object.NewCollection(&object.HashMap{})
	}

	uuid := uuid.New().String()
	tag := &WXTag{
		TempID:       &uuid,
		Name:         mapObject.GetStringPointer("name", ""),
		CreateTime:   mapObject.GetIntPointer("order", -1),
		Order:        mapObject.GetIntPointer("order", -1),
		Deleted:      mapObject.GetBoolPointer("deleted", false),
		WXTagGroupID: mapObject.GetStringPointer("wxTagGroupID", ""),
	}

	return tag
}

func (mdl *WXTag) GetTableName(needFull bool) string {
	tableName := TABLE_NAME_TAG
	if needFull {
		tableName = databasePowerLib.GetTableFullName(config.G_DBConfig.Schemas.Default, config.G_DBConfig.Prefix, tableName)
	}
	return tableName
}

/**
 *  Relationships
 */

/**
 * Scope Where Conditions
 */
func (mdl *WXTag) WhereWXTagName(uuidOrPhone string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("uuid=@value OR mobile=@value", sql.Named("value", uuidOrPhone))
	}
}

func (mdl *WXTag) WhereIsActive(db *gorm.DB) func(db *gorm.DB) *gorm.DB {
	//return db.Where("status = ?", "active")
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("active = ?", true)
	}
}

func (mdl *WXTag) GetTagIDsFromTags(tags []*WXTag) []string {
	tagIDs := []string{}
	for _, tag := range tags {
		tagIDs = append(tagIDs, tag.ID)
	}
	return tagIDs
}
