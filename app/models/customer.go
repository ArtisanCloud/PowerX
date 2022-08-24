package models

import (
	"database/sql"
	"github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	databaseConfig "github.com/ArtisanCloud/PowerX/configs/database"
	"gorm.io/gorm"
)

// TableName overrides the table name used by Customer to `profiles`
func (mdl *Customer) TableName() string {
	return mdl.GetTableName(true)
}

type Customer struct {
	*database.PowerModel

	PivotEmployees []*RCustomerToEmployee `gorm:"ForeignKey:CustomerReferID;references:ExternalUserID" json:"pivotEmployees"`
	FollowUsers    []*Employee            `gorm:"many2many:public.ac_r_customer_to_employee;foreignKey:ExternalUserID;joinForeignKey:CustomerReferID;References:WXUserID;JoinReferences:EmployeeReferID" json:"followUsers"`

	//AnnualMemberships      []*Membership     `gorm:"-" json:"annualMemberships"`
	*wx.WXCustomer
	SessionKey string `gorm:"column:session_key" json:"-"`
}

const TABLE_NAME_ACCOUNT = "customers"
const ACCOUNT_UNIQUE_ID = "external_user_id"

func NewCustomer(mapObject *object.Collection) *Customer {
	if mapObject == nil {
		mapObject = object.NewCollection(&object.HashMap{})
	}

	return &Customer{
		PowerModel: database.NewPowerModel(),
		WXCustomer: wx.NewWXCustomer(mapObject),
	}
}

func (mdl *Customer) GetTableName(needFull bool) string {
	tableName := TABLE_NAME_ACCOUNT
	if needFull {
		tableName = database.GetTableFullName(databaseConfig.G_DBConfig.Schemas["default"], databaseConfig.G_DBConfig.BaseConfig.Prefix, tableName)
	}
	return tableName
}

func (mdl *Customer) GetID() int32 {
	return mdl.ID
}

func (mdl *Customer) GetUUID() string {
	return mdl.UUID
}

func (mdl *Customer) GetForeignRefer() string {
	return ACCOUNT_UNIQUE_ID
}
func (mdl *Customer) GetForeignReferValue() string {
	return mdl.ExternalUserID.String
}

/**
 *  Relationships
 */

/**
 * Scope Where Conditions
 */
func (mdl *Customer) WhereCustomerName(uuidOrPhone string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("uuid=@value OR mobile=@value", sql.Named("value", uuidOrPhone))
	}
}

func (mdl *Customer) WhereOpenID(openID string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("open_id=@value", sql.Named("value", openID))
	}
}

func (mdl *Customer) WhereExternalUserID(externalUserID string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("external_user_id=@value", sql.Named("value", externalUserID))
	}
}

func (mdl *Customer) WhereMobile(phone string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("mobile=@value", sql.Named("value", phone))
	}
}

func (mdl *Customer) WhereIsActive(db *gorm.DB) func(db *gorm.DB) *gorm.DB {
	//return db.Where("status = ?", "active")
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("active = ?", true)
	}
}

/**
 * Association belongings
 */

// -- pivot employees
func (mdl *Customer) LoadPivotEmployees(db *gorm.DB, conditions *map[string]interface{}) ([]*RCustomerToEmployee, error) {
	mdl.PivotEmployees = []*RCustomerToEmployee{}
	result := database.SelectMorphPivots(db, &RCustomerToEmployee{}, true, false).
		Preload("Employees").
		Find(&mdl.PivotEmployees)

	return mdl.PivotEmployees, result.Error
}
