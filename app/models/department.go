package models

import (
	"database/sql"
	databasePowerLib "github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/config"
	"gorm.io/gorm"
)

// TableName overrides the table name used by Department to `profiles`
func (mdl *Department) TableName() string {
	return mdl.GetTableName(true)
}

type Department struct {
	SubDepartments []*Department `gorm:"ForeignKey:ParentID;references:id" json:"subDepartments"`
	Employees      []*Employee   `gorm:"many2many:public.ac_r_employee_to_department;foreignKey:ID;joinForeignKey:department_id;References:WXUserID;JoinReferences:employee_id" json:"employees"`

	*wx.WXDepartment
}

const TABLE_NAME_DEPARTMENT = "departments"
const DEPARTMENT_UNIQUE_ID = "id"

func NewDepartment(mapObject *object.Collection) *Department {
	if mapObject == nil {
		mapObject = object.NewCollection(&object.HashMap{})
	}

	tag := &Department{}

	return tag
}

func (mdl *Department) GetTableName(needFull bool) string {
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
func (mdl *Department) LoadSubDepartments(db *gorm.DB, conditions *map[string]interface{}) ([]*Department, error) {
	mdl.SubDepartments = []*Department{}
	err := databasePowerLib.AssociationRelationship(db, conditions, mdl, "SubDepartments", false).Find(&mdl.SubDepartments)
	if err != nil {
		panic(err)
	}
	return mdl.SubDepartments, err
}

// -- SubDepartments
func (mdl *Department) LoadEmployees(db *gorm.DB, conditions *map[string]interface{}) ([]*Employee, error) {
	mdl.Employees = []*Employee{}
	err := databasePowerLib.AssociationRelationship(db, conditions, mdl, "Employees", false).Find(&mdl.Employees)
	if err != nil {
		panic(err)
	}
	return mdl.Employees, err
}

/**
 * Scope Where Conditions
 */
func (mdl *Department) WhereDepartmentName(uuidOrPhone string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("uuid=@value OR mobile=@value", sql.Named("value", uuidOrPhone))
	}
}

func (mdl *Department) WhereIsActive(db *gorm.DB) func(db *gorm.DB) *gorm.DB {
	//return db.Where("status = ?", "active")
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("active = ?", true)
	}
}
