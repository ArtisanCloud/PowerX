package wx

import (
	"database/sql"
	databasePowerLib "github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/config"
	"gorm.io/gorm"
)

// TableName overrides the table name used by WXDepartment to `profiles`
func (mdl *WXDepartment) TableName() string {
	return mdl.GetTableName(true)
}

type WXDepartment struct {

	//Employees      []*WXEmployee   `gorm:"many2many:public.ac_r_employee_to_department;foreignKey:ID;joinForeignKey:department_id;References:WXUserID;JoinReferences:employee_id" json:"employees"`

	ID       int    `json:"id"`
	Name     string `json:"name"`
	NameEN   string `json:"name_en"`
	ParentID int    `json:"parentid"`
	Order    int    `json:"order"`
}

const TABLE_NAME_WX_DEPARTMENT = "wx_departments"
const DEPARTMENT_UNIQUE_ID = "id"

func NewWXDepartment(mapObject *object.Collection) *WXDepartment {
	if mapObject == nil {
		mapObject = object.NewCollection(&object.HashMap{})
	}

	tag := &WXDepartment{}

	return tag
}

func (mdl *WXDepartment) GetTableName(needFull bool) string {
	tableName := TABLE_NAME_WX_DEPARTMENT
	if needFull {
		tableName = databasePowerLib.GetTableFullName(config.G_DBConfig.Schemas.Default, config.G_DBConfig.Prefix, tableName)
	}
	return tableName
}

/**
 *  Relationships
 */

/**
 * Association belongings
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
