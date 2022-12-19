package models

import (
	"github.com/ArtisanCloud/PowerLibs/v2/authorization/rbac/models"
	"github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerLibs/v2/security"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	databaseConfig "github.com/ArtisanCloud/PowerX/config"
)

// TableName overrides the table name used by Employee to `profiles`
func (mdl *Employee) TableName() string {
	return mdl.GetTableName(true)
}

type Employee struct {
	*database.PowerModel
	UniqueID          string                 `gorm:"column:employee_id;index:,unique" json:"employeeID"`
	Role              *models.Role           `gorm:"ForeignKey:RoleID;references:UniqueID" json:"role"`
	PivotCustomers    []*RCustomerToEmployee `gorm:"ForeignKey:EmployeeReferID;references:WXUserID" json:"pivotCustomers"`
	FollowedEmployees []*Employee            `gorm:"many2many:public.ac_r_customer_to_employee;foreignKey:UUID;joinForeignKey:EmployeeReferID;References:UUID;JoinReferences:EmployeeReferID" json:"followedEmployees"`
	WXDepartments     []*wx.WXDepartment     `gorm:"many2many:ac_r_employee_to_department;foreignKey:WXUserID;joinForeignKey:employee_id;References:ID;JoinReferences:department_id" json:"wxDepartments"`
	//WXTags            []*wechat.WXTag        `gorm:"many2many:public.ac_r_wx_tag_to_object;foreignKey:UUID;joinForeignKey:EmployeeReferID;References:ID;JoinReferences:WXTagReferID" json:"wxTags"`

	RoleID    *string `gorm:"column:role_id;index" json:"roleID"`
	Locale    string  `gorm:"column:locale" json:"locale"`
	Email     string  `gorm:"column:email" json:"email"`
	Password  *string `gorm:"column:password" json:"password"`
	FirstName string  `gorm:"column:firstname" json:"firstname"`
	Lastname  string  `gorm:"column:lastname" json:"lastname"`
	Name      string  `gorm:"column:name" json:"name"`
	Mobile    string  `gorm:"column:mobile" json:"mobile"`

	*wx.WXEmployee
}

const TABLE_NAME_EMPLOYEE = "employees"
const EMPLOYEE_UNIQUE_ID = "employee_id"

func (mdl *Employee) GetTableName(needFull bool) string {
	tableName := TABLE_NAME_EMPLOYEE
	if needFull {
		tableName = database.GetTableFullName(databaseConfig.G_DBConfig.Schemas.Default, databaseConfig.G_DBConfig.Prefix, tableName)
	}
	return tableName
}

func (mdl *Employee) GetForeignRefer() string {
	return EMPLOYEE_UNIQUE_ID
}
func (mdl *Employee) GetForeignReferValue() string {
	return mdl.WXUserID.String
}

func (mdl *Employee) GetComposedUniqueID() string {
	strID := mdl.WXUserID.String + "-" + mdl.WXCorpID.String + "-" + mdl.WXOpenID.String + "-" + mdl.Email + mdl.Mobile
	hashedID := security.HashStringData(strID)

	return hashedID
}

func NewEmployee(mapObject *object.Collection) *Employee {
	if mapObject == nil {
		mapObject = object.NewCollection(&object.HashMap{})
	}

	strDepartments, _ := object.JsonEncode(mapObject.GetFloat64Array("departments", []float64{}))
	strIsLeaderInDept, _ := object.JsonEncode(mapObject.GetFloat64Array("isLeaderInDept", []float64{}))
	strOrder, _ := object.JsonEncode(mapObject.GetInterfaceArray("order", []interface{}{}))

	strUserID := mapObject.GetString("userID", "")
	strCorpID := mapObject.GetString("corpID", "")
	strOpenID := mapObject.GetString("openID", "")
	strEmail := mapObject.GetString("email", "")

	if (strOpenID == "" || strCorpID == "" || strUserID == "") && strEmail == "" {
		return nil
	}

	userID := object.NewNullString(strUserID, true)
	corpID := object.NewNullString(strCorpID, true)
	openID := object.NewNullString(strOpenID, true)

	newEmployee := &Employee{
		PowerModel: database.NewPowerModel(),

		RoleID:    mapObject.GetStringPointer("roleID", ""),
		Email:     strEmail,
		Password:  mapObject.GetStringPointer("password", ""),
		FirstName: mapObject.GetString("firstName", ""),
		Lastname:  mapObject.GetString("lastName", ""),
		Name:      mapObject.GetString("name", ""),
		Mobile:    mapObject.GetString("mobile", ""),

		WXEmployee: &wx.WXEmployee{
			WXAlias:           mapObject.GetString("alias", ""),
			WXAvatar:          mapObject.GetString("avatar", ""),
			WXDepartment:      strDepartments,
			WXEmail:           mapObject.GetString("email", ""),
			WXEnable:          int(mapObject.GetFloat64("enable", 0)),
			WXEnglishName:     mapObject.GetString("englishName", ""),
			WXExtAttr:         mapObject.GetString("extAttr", ""),
			WXExternalProfile: mapObject.GetString("externalProfile", ""),
			WXGender:          mapObject.GetString("gender", ""),
			WXHideMobile:      int(mapObject.GetFloat64("hideMobile", 0)),
			WXIsLeader:        int(mapObject.GetFloat64("isLeader", 0)),
			WXIsLeaderInDept:  strIsLeaderInDept,
			WXMainDepartment:  int(mapObject.GetFloat64("mainDepartment", 0)),
			WXMobile:          mapObject.GetString("mobile", ""),
			WXName:            mapObject.GetString("name", ""),
			WXOrder:           strOrder,
			WXPosition:        mapObject.GetString("position", ""),
			WXQrCode:          mapObject.GetString("contactWay", ""),
			WXStatus:          int(mapObject.GetFloat64("status", 0)),
			WXTelephone:       mapObject.GetString("telephone", ""),
			WXThumbAvatar:     mapObject.GetString("thumbAvatar", ""),
			WXUserID:          userID,
			WXCorpID:          corpID,
			WXOpenID:          openID,
		},
	}

	newEmployee.UniqueID = newEmployee.GetComposedUniqueID()

	return newEmployee
}

func (mdl *Employee) GetEmployeeUUIDsFromEmployees(employees []*Employee) []string {
	employeeIDs := []string{}
	for _, employee := range employees {
		employeeIDs = append(employeeIDs, employee.UUID)
	}
	return employeeIDs
}

func (mdl *Employee) GetEmployeeUserIDsFromEmployees(employees []*Employee) []string {
	employeeUserIDs := []string{}
	for _, employee := range employees {
		employeeUserIDs = append(employeeUserIDs, employee.WXUserID.String)
	}
	return employeeUserIDs
}

/**
 * Scope Where Conditions
 */
