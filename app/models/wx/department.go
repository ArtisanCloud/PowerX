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
	SubDepartments []*WXDepartment `gorm:"ForeignKey:ParentID;references:id" json:"subDepartments"`

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

// -- SubDepartments
func (mdl *WXDepartment) LoadSubDepartments(db *gorm.DB, conditions *map[string]interface{}) ([]*WXDepartment, error) {
	mdl.SubDepartments = []*WXDepartment{}
	err := databasePowerLib.AssociationRelationship(db, conditions, mdl, "SubDepartments", false).Find(&mdl.SubDepartments)
	if err != nil {
		panic(err)
	}
	return mdl.SubDepartments, err
}

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
