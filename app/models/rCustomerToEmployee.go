package models

import (
	"errors"
	"github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerLibs/v2/security"
	"github.com/ArtisanCloud/PowerSocialite/v2/src/models"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/config"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TableName overrides the table name used by RCustomerToEmployee to `profiles`
func (mdl *RCustomerToEmployee) TableName() string {
	return mdl.GetTableName(true)
}

// r_customer_to_employee 数据表结构
type RCustomerToEmployee struct {
	*database.PowerPivot

	PivotWXTags []*wx.RWXTagToObject `gorm:"ForeignKey:TaggableObjectID;references:UniqueID" json:"pivotWXTags"`

	//common fields
	UniqueID        string            `gorm:"index:index_employee_refer_id;index:index_customer_refer_id;index;column:index_customer_to_employee_id;unique"`
	EmployeeReferID object.NullString `gorm:"column:employee_refer_id;not null;index:index_employee_refer_id" json:"employeeReferID"`
	CustomerReferID object.NullString `gorm:"column:customer_refer_id;not null;index:index_customer_refer_id" json:"customerReferID"`

	AddWay         *int           `json:"add_way"`
	CreateTime     *int           `json:"createtime"`
	Description    *string        `json:"description"`
	OperatorUserID *string        `json:"oper_userid"`
	Remark         *string        `json:"remark"`
	RemarkMobiles  datatypes.JSON `json:"remark_mobiles"`
	State          *string        `json:"state"`
	UserID         *string        `json:"userid"`
	RemarkCorpName *string        `json:"remark_corp_name"`
	WechatChannels datatypes.JSON `json:"wechat_channels"`
}

const TABLE_NAME_R_CUSTOMER_TO_EMPLOYEE = "r_customer_to_employee"
const R_CUSTOMER_TO_EMPLOYEE_UNIQUE_ID = "index_customer_to_employee_id"

const R_CUSTOMER_TO_EMPLOYEE_FOREIGN_KEY = "customer_refer_id"
const R_CUSTOMER_TO_EMPLOYEE_JOIN_KEY = "employee_refer_id"

func (mdl *RCustomerToEmployee) GetTableName(needFull bool) string {
	tableName := TABLE_NAME_R_CUSTOMER_TO_EMPLOYEE
	if needFull {
		tableName = config.DatabaseConn.Schemas["option"] + "." + tableName
	}
	return tableName
}

func (mdl *RCustomerToEmployee) GetForeignRefer() string {
	return R_CUSTOMER_TO_EMPLOYEE_UNIQUE_ID
}
func (mdl *RCustomerToEmployee) GetForeignReferValue() string {
	return mdl.UniqueID
}

func (mdl *RCustomerToEmployee) GetForeignKey() string {
	return R_CUSTOMER_TO_EMPLOYEE_FOREIGN_KEY
}

func (mdl *RCustomerToEmployee) GetForeignValue() string {
	return mdl.CustomerReferID.String
}

func (mdl *RCustomerToEmployee) GetJoinKey() string {
	return R_CUSTOMER_TO_EMPLOYEE_JOIN_KEY
}

func (mdl *RCustomerToEmployee) GetJoinValue() string {
	return mdl.EmployeeReferID.String
}

func (mdl *RCustomerToEmployee) GetPivotComposedUniqueID() string {
	strID := mdl.GetForeignValue() + "-" + mdl.GetJoinValue()
	hashedID := security.HashStringData(strID)

	return hashedID
}

func (mdl *RCustomerToEmployee) UpsertPivotByFollowUser(db *gorm.DB, customer *Customer, followUser *models.FollowUser) (pivot *RCustomerToEmployee, err error) {

	remarkMobiles, err := object.JsonEncode(followUser.RemarkMobiles)
	if err != nil {
		return nil, err
	}

	wechatChannels, err := object.JsonEncode(followUser.WechatChannels)
	if err != nil {
		return nil, err
	}

	pivot = &RCustomerToEmployee{

		EmployeeReferID: object.NewNullString(followUser.UserID, true),
		CustomerReferID: customer.ExternalUserID,
		AddWay:          &followUser.AddWay,
		CreateTime:      &followUser.CreateTime,
		Description:     &followUser.Description,
		OperatorUserID:  &followUser.OperUserID,
		Remark:          &followUser.Remark,
		RemarkMobiles:   datatypes.JSON(remarkMobiles),
		State:           &followUser.State,
		UserID:          &followUser.UserID,
		RemarkCorpName:  &followUser.RemarkCorpName,
		WechatChannels:  datatypes.JSON(wechatChannels),
	}
	pivot.UniqueID = pivot.GetPivotComposedUniqueID()

	err = mdl.UpsertPivots(db, []*RCustomerToEmployee{pivot}, nil)

	return pivot, err
}

func (mdl *RCustomerToEmployee) UpsertPivots(db *gorm.DB, pivots []*RCustomerToEmployee, fieldsToUpdate []string) error {

	if len(pivots) <= 0 {
		return nil
	}

	if len(fieldsToUpdate) <= 0 {
		fieldsToUpdate = database.GetModelFields(&RCustomerToEmployee{})
	}

	result := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: R_CUSTOMER_TO_EMPLOYEE_UNIQUE_ID}},
		DoUpdates: clause.AssignmentColumns(fieldsToUpdate),
	}).Create(&pivots)

	return result.Error
}

func (mdl *RCustomerToEmployee) ClearPivot(db *gorm.DB, customerExternalUserID string, employeeUserID string) (*RCustomerToEmployee, error) {
	mdl.CustomerReferID = object.NewNullString(customerExternalUserID, true)
	mdl.EmployeeReferID = object.NewNullString(employeeUserID, true)

	err := database.ClearPivots(db, mdl, true, false)

	return mdl, err
}

func (mdl *RCustomerToEmployee) ConvertCustomerUserIDs(pivots []*RCustomerToEmployee) (customerIDs []string) {

	for _, pivot := range pivots {
		customerIDs = append(customerIDs, pivot.CustomerReferID.String)
	}
	return customerIDs
}

func (mdl *RCustomerToEmployee) ConvertEmployUserIDs(pivots []*RCustomerToEmployee) (employeeIDs []string) {

	for _, pivot := range pivots {
		employeeIDs = append(employeeIDs, pivot.EmployeeReferID.String)
	}
	return employeeIDs
}

func (mdl *RCustomerToEmployee) GetPivotsByCustomerUserID(db *gorm.DB, customerExternalUserID string) ([]*RCustomerToEmployee, error) {

	pivots := []*RCustomerToEmployee{}

	mdl.CustomerReferID = object.NewNullString(customerExternalUserID, true)

	result := database.SelectPivots(db, mdl, true, false).Find(&pivots)

	if result.Error != nil {
		return nil, result.Error
	}
	return pivots, result.Error
}

func (mdl *RCustomerToEmployee) GetPivotsByEmployeeUserID(db *gorm.DB, employeeUserID string) ([]*RCustomerToEmployee, error) {
	pivots := []*RCustomerToEmployee{}

	mdl.EmployeeReferID = object.NewNullString(employeeUserID, true)

	result := database.SelectPivots(db, mdl, false, true).Find(&pivots)

	if result.Error != nil {
		return nil, result.Error
	}

	return pivots, result.Error
}

func (mdl *RCustomerToEmployee) GetPivot(db *gorm.DB, customerExternalUserID string, employeeUserID string) (*RCustomerToEmployee, error) {

	mdl.CustomerReferID = object.NewNullString(customerExternalUserID, true)
	mdl.EmployeeReferID = object.NewNullString(employeeUserID, true)

	result := database.SelectPivot(db, mdl).First(mdl)

	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, result.Error
	}
	if result.RowsAffected == 0 || result.Error == gorm.ErrRecordNotFound {
		return nil, nil
	}

	return mdl, result.Error
}
