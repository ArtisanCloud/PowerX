package wx

import (
	"database/sql"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/config/database"
	"gorm.io/gorm"
)

// TableName overrides the table name used by WXDepartment to `profiles`
func (mdl *WXDepartment) TableName() string {
	return mdl.GetTableName(true)
}

type WXDepartment struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	NameEN   string `json:"name_en"`
	ParentID int    `json:"parentid"`
	Order    int    `json:"order"`
}

const TABLE_NAME_DEPARTMENT = "wx_departments"
const DEPARTMENT_UNIQUE_ID = "id"

func NewWXDepartment(mapObject *object.Collection) *WXDepartment {
	if mapObject == nil {
		mapObject = object.NewCollection(&object.HashMap{})
	}

	tag := &WXDepartment{}

	return tag
}

func (mdl *WXDepartment) GetTableName(needFull bool) string {
	tableName := TABLE_NAME_DEPARTMENT
	if needFull {
		tableName = database.G_DBConfig.Schemas["option"] + "." + tableName
	}
	return tableName
}

/**
 *  Relationships
 */

/**
 * Scope Where Conditions
 */
func (mdl *WXDepartment) WhereWXDepartmentName(uuidOrPhone string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("uuid=@value OR mobile=@value", sql.Named("value", uuidOrPhone))
	}
}

func (mdl *WXDepartment) WhereIsActive(db *gorm.DB) func(db *gorm.DB) *gorm.DB {
	//return db.Where("status = ?", "active")
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("active = ?", true)
	}
}
