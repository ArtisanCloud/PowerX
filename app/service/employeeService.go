package service

import (
	"encoding/json"
	"errors"
	databasePowerLib "github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/fmt"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	modelSocialite "github.com/ArtisanCloud/PowerSocialite/v2/src/models"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/kernel/contract"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/work/externalContact/tag/request"
	modelPowerWechat "github.com/ArtisanCloud/PowerWeChat/v2/src/work/server/handlers/models"
	"github.com/ArtisanCloud/PowerX/app/models"
	modelWX "github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/app/service/wx/wecom"
	"github.com/ArtisanCloud/PowerX/database/global"
	logger "github.com/ArtisanCloud/PowerX/loggerManager"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type EmployeeService struct {
	Service  *Service
	Employee *models.Employee
}

/**
 ** 初始化构造函数
 */

// 模块初始化函数 import 包时被调用
func init() {
	//fmt.Println("Employee service module init function")
}

func NewEmployeeService(ctx *gin.Context) (r *EmployeeService) {
	r = &EmployeeService{
		Service:  NewService(ctx),
		Employee: models.NewEmployee(nil),
	}
	return r
}

func (srv *EmployeeService) SyncEmployees() (err error) {

	// get root department
	response, err := wecom.G_WeComEmployee.App.User.GetDetailedDepartmentUsers(1, 1)
	if response.ErrCode != 0 {
		return errors.New(response.ErrMSG)
	}

	strCorpID := wecom.G_WeComEmployee.App.Config.GetString("corp_id", "")
	if strCorpID == "" {
		return errors.New("corp id is empty")
	}

	// parse the result of employees from wx

	serviceWeComEmployee := wecom.NewWeComEmployeeService(nil)
	for _, userDetail := range response.UserList {
		// get employees from wx
		responseOpenID, err := wecom.G_WeComEmployee.App.User.UserIdToOpenID(userDetail.UserID)
		if err != nil {
			return err
		}
		userDetail.OpenID = responseOpenID.OpenID
		userDetail.CorpID = strCorpID
		employee := srv.NewEmployeeFromWXEmployee(userDetail)
		//arrayEmployees = append(arrayEmployees, employee)

		//time.Sleep(time.Second * 30)
		// batch upsert employees
		err = serviceWeComEmployee.UpsertEmployeeByWXEmployee(global.G_DBConnection, employee.WXEmployee)
	}

	return err
}

func (srv *EmployeeService) SyncDepartmentIDsToEmployee(db *gorm.DB, employee *models.Employee, departmentIDs []int) (err error) {
	pivots, err := (&models.REmployeeToDepartment{}).MakePivotsFromEmployeeAndDepartmentIDs(employee, departmentIDs)
	if err != nil {
		return err
	}
	err = databasePowerLib.SyncPivots(db, pivots)
	return err
}

func (srv *EmployeeService) GetList(db *gorm.DB, conditions *map[string]interface{}, page int, pageSize int) (pagination *databasePowerLib.Pagination, err error) {

	arrayEmployees := []*models.Employee{}
	pagination, err = databasePowerLib.GetList(db, conditions, &arrayEmployees, nil, page, pageSize)

	return pagination, err
}

func (srv *EmployeeService) GetAllEmployees(db *gorm.DB, conditions *map[string]interface{}) (arrayEmployees []*models.Employee, err error) {

	arrayEmployees = []*models.Employee{}

	err = databasePowerLib.GetAllList(db, conditions, &arrayEmployees, nil)

	return arrayEmployees, err
}

func (srv *EmployeeService) UpsertEmployees(db *gorm.DB, uniqueName string, employees []*models.Employee) error {

	if len(employees) <= 0 {
		return nil
	}

	result := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: uniqueName}},
		DoUpdates: clause.AssignmentColumns(databasePowerLib.GetModelFields(models.Employee{})),
	}).Create(&employees)

	return result.Error
}

func (srv *EmployeeService) UpsertEmployee(db *gorm.DB, employee *models.Employee) (savedEmployee *models.Employee, err error) {

	employee.UpdatedAt = time.Now()
	if employee.UUID == "" {
		employee.UUID = uuid.NewString()
		employee.CreatedAt = time.Now()
		savedEmployee, err = srv.SaveEmployee(db, employee)
	} else {
		savedEmployee, err = srv.UpdateEmployee(db, employee)
	}

	return savedEmployee, err
}

func (srv *EmployeeService) SaveEmployee(db *gorm.DB, employee *models.Employee) (*models.Employee, error) {
	db = db.Save(employee)

	return employee, db.Error
}

func (srv *EmployeeService) UpdateEmployee(db *gorm.DB, employee *models.Employee) (*models.Employee, error) {

	// clear relationship between from employee to department
	global.G_DBConnection.Where("employee_id=?", employee.ID).Delete(models.REmployeeToDepartment{})

	db = db.Updates(employee)

	return employee, db.Error
}

func (srv *EmployeeService) DeleteEmployees(db *gorm.DB, employee []*models.Employee) error {

	db = db.Delete(employee)

	return db.Error
}

func (srv *EmployeeService) DeleteEmployee(db *gorm.DB, employee *models.Employee) error {

	db = db.Delete(employee)

	return db.Error
}

func (srv *EmployeeService) GetEmployeeUserIDs(db *gorm.DB) (userIDs []string, err error) {

	result := db.
		//Debug().
		Model(srv.Employee).
		Pluck("wx_user_id", &userIDs)

	return userIDs, result.Error

}

func (srv *EmployeeService) GetEmployeesByUserIDs(db *gorm.DB, userIDs []string) (employees []*models.Employee, err error) {

	employees = []*models.Employee{}

	db = db.
		Preload("WXDepartments").
		Where("wx_user_id in (?)", userIDs)
	result := db.Find(&employees)
	return employees, result.Error
}

func (srv *EmployeeService) GetEmployeeByUserID(db *gorm.DB, userID string) (employee *models.Employee, err error) {

	employee = &models.Employee{}

	preloads := []string{"WXDepartments"}

	condition := &map[string]interface{}{
		"wx_user_id": userID,
	}
	err = databasePowerLib.GetFirst(db, condition, employee, preloads)
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return employee, err

}

func (srv *EmployeeService) GetEmployeeByUserIDOnWXPlatform(ctx *gin.Context, userID string) (employee *models.Employee, err error) {
	responseGetEmployeeByID, err := wecom.G_WeComEmployee.App.OAuth.Provider.Detailed().GetUserByID(userID)
	if err != nil {
		return nil, err
	}
	if responseGetEmployeeByID.ErrCode == 0 {
		serviceEmployee := NewEmployeeService(ctx)
		employee = serviceEmployee.NewEmployeeFromWXEmployee(responseGetEmployeeByID.Employee)

	} else {
		return nil, errors.New(responseGetEmployeeByID.ErrMSG)
	}
	return employee, nil
}

func (srv *EmployeeService) NewEmployeeFromWXEmployee(wxEmployee *modelSocialite.Employee) *models.Employee {

	arrayDepartmentIDs := wxEmployee.Department
	//wxDepartments, _ := json.Marshal(arrayDepartmentIDs)
	wxIsLeaderInDept, _ := json.Marshal(wxEmployee.IsLeaderInDept)
	wxOrder, _ := json.Marshal(wxEmployee.Order)
	//wxExtAttr, _ := json.Marshal(wxEmployee.ExtAttr)
	//wxExternalProfile, _ := json.Marshal(wxEmployee.ExternalProfile)

	employee := &models.Employee{
		PowerModel: &databasePowerLib.PowerModel{

			UUID: uuid.New().String(),
			//CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},

		Locale: LOCALE_CN,
		Email:  wxEmployee.Email,
		Name:   wxEmployee.Name,
		Mobile: wxEmployee.Mobile,

		WXEmployee: &modelWX.WXEmployee{

			WXAlias:  wxEmployee.Alias,
			WXAvatar: wxEmployee.Avatar,
			//WXDepartments:,
			WXEmail:       wxEmployee.Email,
			WXEnable:      wxEmployee.Enable,
			WXEnglishName: wxEmployee.EnglishName,
			//WXExtAttr:         string(wxExtAttr),
			//WXExternalProfile: string(wxExternalProfile),
			WXGender:         wxEmployee.Gender,
			WXHideMobile:     wxEmployee.HideMobile,
			WXIsLeaderInDept: string(wxIsLeaderInDept),
			WXIsLeader:       wxEmployee.IsLeader,
			WXMainDepartment: wxEmployee.MainDepartment,
			WXMobile:         wxEmployee.Mobile,
			WXName:           wxEmployee.Name,
			WXOrder:          string(wxOrder),
			WXPosition:       wxEmployee.Position,
			WXQrCode:         wxEmployee.QrCode,
			WXStatus:         wxEmployee.Status,
			WXTelephone:      wxEmployee.Telephone,
			WXThumbAvatar:    wxEmployee.ThumbAvatar,
			WXUserID:         object.NewNullString(wxEmployee.UserID, true),
			WXOpenUserID:     object.NewNullString(wxEmployee.OpenUserID, true),
			WXOpenID:         object.NewNullString(wxEmployee.OpenID, true),
			WXCorpID:         object.NewNullString(wxEmployee.CorpID, true),
		},
	}

	// attach departments to employee
	serviceDepartment := NewDepartmentService(nil)
	arrayDepartments, err := serviceDepartment.GetDepartmentsByIDs(global.G_DBConnection, arrayDepartmentIDs)
	if err == nil {
		employee.WXDepartments = arrayDepartments
	}

	return employee
}

// --------

func (srv *EmployeeService) HandleAddCustomer(context *gin.Context, event contract.EventInterface) (err error) {

	serviceWXTag := wecom.NewWXTagService(context)

	msg := &modelPowerWechat.EventExternalUserAdd{}
	err = event.ReadMessage(msg)
	if err != nil {
		return err
	}

	logger.Logger.Info("Handle Add External Contact", zap.Any("msg", msg))

	// --------------------------------------------------
	// sync customer from wx
	rs, err := wecom.G_WeComApp.App.ExternalContact.Get(msg.ExternalUserID, "")
	if err != nil {
		return err
	}
	serviceCustomer := NewCustomerService(context)
	customer := serviceCustomer.NewCustomerFromWXContact(rs.ExternalContact)
	err = serviceCustomer.UpsertCustomers(global.G_DBConnection, []*models.Customer{customer}, nil)
	if err != nil {
		return err
	}

	// --------------------------------------------------
	// load contact way
	serviceContactWay := NewContactWayService(nil)
	contactWay, err := serviceContactWay.GetContactWayByState(global.G_DBConnection, msg.State)
	if err != nil {
		return err
	}
	tagIDs := []string{}
	if contactWay != nil {
		contactWay.WXTags, err = contactWay.LoadWXTags(global.G_DBConnection, nil)
		if err != nil {
			return err
		}
		tagIDs = serviceWXTag.WXTag.GetTagIDsFromTags(contactWay.WXTags)

	}

	// bind customer to employee
	for _, followInfo := range rs.FollowUsers {
		pivot, err := srv.BindCustomerToEmployee(msg.ExternalUserID, followInfo)
		if err != nil {
			return err
		}
		// save operation log
		_ = (&databasePowerLib.PowerOperationLog{}).SaveOps(global.G_DBConnection, customer.Name, customer,
			MODULE_CUSTOMER, "外部联系人绑定员工", databasePowerLib.OPERATION_EVENT_CREATE,
			pivot.EmployeeReferID.String, pivot, databasePowerLib.OPERATION_RESULT_SUCCESS)

		// upload sync wx platform tags
		req := &request.RequestTagMarkTag{
			UserID:         pivot.EmployeeReferID.String,
			ExternalUserID: pivot.CustomerReferID.String,
			AddTag:         tagIDs,
			RemoveTag:      []string{},
		}
		_, err = wecom.G_WeComCustomer.App.ExternalContactTag.MarkTag(req)

		err = serviceWXTag.SyncWXTagsToObject(global.G_DBConnection, pivot, contactWay.WXTags)
		if err != nil {
			return err
		}
	}

	// --------------------------------------------------
	err = wecom.G_WeComApp.SendAddCustomerWelcomeMsg(context, contactWay, msg)
	if err != nil {
		return err
	}

	return err
}

// -------------------------------------------------------------------------------

func (srv *EmployeeService) HandleEditCustomer(context *gin.Context, event contract.EventInterface) (err error) {
	msg := &modelPowerWechat.EventExternalUserEdit{}
	err = event.ReadMessage(msg)
	if err != nil {
		return err
	}

	logger.Logger.Info("Handle Edit External Contact", zap.Any("msg", msg))

	// --------------------------------------------------
	// sync customer from wx
	rs, err := wecom.G_WeComApp.App.ExternalContact.Get(msg.ExternalUserID, "")
	if err != nil {
		return err
	}
	serviceCustomer := NewCustomerService(context)
	customer := serviceCustomer.NewCustomerFromWXContact(rs.ExternalContact)
	err = serviceCustomer.UpsertCustomerByWXCustomer(global.G_DBConnection, customer.WXCustomer)
	if err != nil {
		return err
	}

	// get employee from event
	employee, err := srv.GetEmployeeByUserID(global.G_DBConnection, msg.UserID)
	if err != nil {
		return err
	}

	// save operation log
	_ = (&databasePowerLib.PowerOperationLog{}).SaveOps(global.G_DBConnection, employee.Name, employee,
		MODULE_CUSTOMER, "员工修改外部联系人", databasePowerLib.OPERATION_EVENT_UPDATE,
		customer.Name, customer, databasePowerLib.OPERATION_RESULT_SUCCESS)

	// sync wx tags to customer
	if len(rs.FollowUsers) > 0 {
		for _, followInfo := range rs.FollowUsers {
			pivot, err := (&models.RCustomerToEmployee{}).UpsertPivotByFollowUser(global.G_DBConnection, customer, followInfo)
			if err != nil {
				fmt.Dump(err.Error())
				continue
			}
			if len(followInfo.Tags) > 0 {
				serviceWXTag := wecom.NewWXTagService(nil)
				err = serviceWXTag.SyncWXTagsByFollowInfos(global.G_DBConnection, pivot, followInfo)

			}
		}
	}

	return err
}
func (srv *EmployeeService) HandleAddHalfCustomer(context *gin.Context, event contract.EventInterface) (err error) {

	msg := &modelPowerWechat.EventExternalUserAddHalf{}
	err = event.ReadMessage(&msg)
	if err != nil {
		return err
	}

	logger.Logger.Info("Handle Add Half External Contact", zap.Any("msg", msg))

	return err
}
func (srv *EmployeeService) HandleDelCustomer(context *gin.Context, event contract.EventInterface) (err error) {

	msg := &modelPowerWechat.EventExternalUserDel{}
	err = event.ReadMessage(&msg)
	if err != nil {
		return err
	}

	logger.Logger.Info("Handle Del External Contact", zap.Any("msg", msg))

	// unbind customer from employee
	customer, employee, err := srv.UnbindCustomerToEmployee(msg.ExternalUserID, msg.UserID)
	if err != nil {
		wecom.G_WeComApp.App.Logger.Error(err.Error())
		return err
	}
	// save operation log
	_ = (&databasePowerLib.PowerOperationLog{}).SaveOps(global.G_DBConnection, employee.Name, employee,
		MODULE_CUSTOMER, "员工删除外部联系人", databasePowerLib.OPERATION_EVENT_DELETE,
		customer.Name, customer, databasePowerLib.OPERATION_RESULT_SUCCESS)

	//fmt.Dump(msg)

	return err

}
func (srv *EmployeeService) HandleDelFollowEmployee(context *gin.Context, event contract.EventInterface) (msg *modelPowerWechat.EventExternalUserDelFollowUser, err error) {
	msg = &modelPowerWechat.EventExternalUserDelFollowUser{}
	err = event.ReadMessage(&msg)
	if err != nil {
		return msg, err
	}
	logger.Logger.Info("Handle Del Follow User", zap.Any("msg", msg))

	// unbind customer from employee
	customer, employee, err := srv.UnbindCustomerToEmployee(msg.ExternalUserID, msg.UserID)
	if err != nil {
		wecom.G_WeComApp.App.Logger.Error(err.Error())
		return msg, err
	}
	// save operation log
	_ = (&databasePowerLib.PowerOperationLog{}).SaveOps(global.G_DBConnection, customer.Name, customer,
		MODULE_CUSTOMER, "外部联系人删除员工", databasePowerLib.OPERATION_EVENT_DELETE,
		employee.Name, employee, databasePowerLib.OPERATION_RESULT_SUCCESS)
	//fmt.Dump(msg)

	return msg, err

}
func (srv *EmployeeService) HandleTransferFail(context *gin.Context, event contract.EventInterface) (err error) {
	fmt.Dump("Handle Transfer Fail")
	return err
}

// --------------------------------------------------
// handle events from WeComApp Contact
// --------------------------------------------------

func (srv *EmployeeService) HandleEmployeeCreate(context *gin.Context, event contract.EventInterface) (err error) {

	msg := &modelPowerWechat.EventExternalUserAdd{}
	err = event.ReadMessage(msg)
	if err != nil {
		return err
	}

	logger.Logger.Info("Handle Create Contact", zap.Any("msg", msg))

	return err
}

func (srv *EmployeeService) HandleEmployeeUpdate(context *gin.Context, event contract.EventInterface) (err error) {

	msg := &modelPowerWechat.EventExternalUserAdd{}
	err = event.ReadMessage(msg)
	if err != nil {
		return err
	}

	logger.Logger.Info("Handle Update Contact", zap.Any("msg", msg))

	return err
}

func (srv *EmployeeService) HandleEmployeeDelete(context *gin.Context, event contract.EventInterface) (err error) {

	msg := &modelPowerWechat.EventExternalUserAdd{}
	err = event.ReadMessage(msg)
	if err != nil {
		return err
	}

	logger.Logger.Info("Handle Delete Contact", zap.Any("msg", msg))

	return err
}

// --------------------------------------------------
// handle events from WeComApp contact Tag
// --------------------------------------------------

func (srv *EmployeeService) HandleContactTagUpdate(context *gin.Context, event contract.EventInterface) (err error) {

	msg := &modelPowerWechat.EventExternalUserAdd{}
	err = event.ReadMessage(msg)
	if err != nil {
		return err
	}

	logger.Logger.Info("Handle Create Contact", zap.Any("msg", msg))

	return err
}

// --------------------------------------------------
func (srv *EmployeeService) BindCustomerToEmployee(UserExternalID string, followInfo *modelSocialite.FollowUser) (pivot *models.RCustomerToEmployee, err error) {
	serviceCustomer := NewCustomerService(nil)

	// get customer from event
	customer, err := serviceCustomer.GetCustomerByExternalUserID(global.G_DBConnection, UserExternalID)
	if err != nil {
		return pivot, err
	}

	// follow customer to employees
	pivot, err = serviceCustomer.SyncFollowEmployee(global.G_DBConnection, customer, followInfo)
	//_, err = serviceCustomer.FollowEmployees(global.G_DBConnection, customer, followInfos)
	if err != nil {
		return pivot, err
	}

	return pivot, err

}

func (srv *EmployeeService) UnbindCustomerToEmployee(UserExternalID string, UserID string) (customer *models.Customer, employee *models.Employee, err error) {

	serviceWXTag := wecom.NewWXTagService(nil)
	serviceCustomer := NewCustomerService(nil)

	// get employee from event
	employee, err = srv.GetEmployeeByUserID(global.G_DBConnection, UserID)
	if err != nil {
		return customer, employee, err
	}

	// get customer from event
	customer, err = serviceCustomer.GetCustomerByExternalUserID(global.G_DBConnection, UserExternalID)
	if err != nil {
		return customer, employee, err
	}

	// clear tags from RWXCustomerToEmployee
	pivot, err := (&models.RCustomerToEmployee{}).GetPivot(global.G_DBConnection, UserExternalID, UserID)
	if err != nil {
		return customer, employee, err
	}
	if pivot == nil {
		return nil, nil, errors.New("pivot not found")
	}

	err = serviceWXTag.ClearObjectWXTags(global.G_DBConnection, pivot)
	if err != nil {
		return customer, employee, err
	}

	// detach customer to employees
	err = serviceCustomer.UnfollowEmployee(global.G_DBConnection, customer, []*models.Employee{employee})
	if err != nil {
		return customer, employee, err
	}

	return customer, employee, err

}
