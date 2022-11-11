package service

import (
	"encoding/json"
	"errors"
	models2 "github.com/ArtisanCloud/PowerLibs/v2/authorization/rbac/models"
	databasePowerLib "github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/fmt"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	modelSocialite "github.com/ArtisanCloud/PowerSocialite/v2/src/models"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/kernel/contract"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/work/externalContact/tag/request"
	modelWecomEvent "github.com/ArtisanCloud/PowerWeChat/v2/src/work/server/handlers/models"
	"github.com/ArtisanCloud/PowerX/app/models"
	modelWX "github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/app/service/wx/weCom"
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

func (srv *EmployeeService) SyncEmployees(departmentID int, fetchChild int) (err error) {

	// get root department
	response, err := weCom.G_WeComEmployee.App.User.GetDetailedDepartmentUsers(departmentID, fetchChild)
	if response.ErrCode != 0 {
		return errors.New(response.ErrMSG)
	}

	strCorpID := weCom.G_WeComEmployee.App.Config.GetString("corp_id", "")
	if strCorpID == "" {
		return errors.New("corp id is empty")
	}

	// parse the result of employees from wechat

	serviceWeComEmployee := weCom.NewWeComEmployeeService(nil)
	for _, userDetail := range response.UserList {
		// get employees from wechat
		responseOpenID, err := weCom.G_WeComEmployee.App.User.UserIdToOpenID(userDetail.UserID)
		if err != nil {
			return err
		}
		userDetail.OpenID = responseOpenID.OpenID
		userDetail.CorpID = strCorpID
		employee := srv.NewEmployeeFromWXEmployee(userDetail)
		//arrayEmployees = append(arrayEmployees, employee)

		//time.Sleep(time.Second * 30)
		// batch upsert employees
		err = global.G_DBConnection.Transaction(func(tx *gorm.DB) error {
			err = serviceWeComEmployee.UpsertEmployeeByWXEmployee(tx, employee)
			// upsert associates
			if len(employee.WXEmployee.WXDepartment) > 0 {
				departmentIDs := []int{}
				err = object.JsonDecode([]byte(employee.WXEmployee.WXDepartment), &departmentIDs)
				if err != nil {
					return err
				}
				err = srv.SyncDepartmentIDsToEmployee(tx, employee, departmentIDs)

			}
			return err
		})
		if err != nil {
			logger.Logger.Error(err.Error())
			continue
		}
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

	preloads := []string{"WXDepartments", "Role"}

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
	responseGetEmployeeByID, err := weCom.G_WeComEmployee.App.OAuth.Provider.Detailed().GetUserByID(userID)
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
	wxDepartments, _ := json.Marshal(arrayDepartmentIDs)
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

			WXAlias:       wxEmployee.Alias,
			WXAvatar:      wxEmployee.Avatar,
			WXDepartment:  string(wxDepartments),
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

	serviceWXTag := weCom.NewWXTagService(context)

	msg := &modelWecomEvent.EventExternalUserAdd{}
	err = event.ReadMessage(msg)
	if err != nil {
		return err
	}

	logger.Logger.Info("Handle Add External Contact", zap.Any("msg", msg))

	// --------------------------------------------------
	// 同步客户的信息
	rs, err := weCom.G_WeComApp.App.ExternalContact.Get(msg.ExternalUserID, "")
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
	// 加载客户联系
	serviceContactWay := NewContactWayService(nil)
	contactWay, err := serviceContactWay.GetContactWayByState(global.G_DBConnection, msg.State)
	if err != nil {
		return err
	}

	// 获取客户联系配置的微信标签
	tagIDs := []string{}
	if contactWay != nil {
		contactWay.WXTags, err = contactWay.LoadWXTags(global.G_DBConnection, nil)
		if err != nil {
			return err
		}
		tagIDs = serviceWXTag.WXTag.GetTagIDsFromTags(contactWay.WXTags)
	}

	for _, followInfo := range rs.FollowUsers {
		// 绑定客户与员工关系
		pivot, err := srv.BindCustomerToEmployee(msg.ExternalUserID, followInfo)
		if err != nil {
			return err
		}
		// 保存操作日志
		_ = (&databasePowerLib.PowerOperationLog{}).SaveOps(global.G_DBConnection, customer.Name, customer,
			MODULE_CUSTOMER, "外部联系人绑定员工", databasePowerLib.OPERATION_EVENT_CREATE,
			pivot.EmployeeReferID.String, pivot, databasePowerLib.OPERATION_RESULT_SUCCESS)

		// 上传同步微信平台的微信标签
		req := &request.RequestTagMarkTag{
			UserID:         pivot.EmployeeReferID.String,
			ExternalUserID: pivot.CustomerReferID.String,
			AddTag:         tagIDs,
			RemoveTag:      []string{},
		}
		_, err = weCom.G_WeComCustomer.App.ExternalContactTag.MarkTag(req)

		err = serviceWXTag.SyncWXTagsToObject(global.G_DBConnection, pivot, contactWay.WXTags)
		if err != nil {
			return err
		}
	}

	// --------------------------------------------------
	// 发送在联系客户中配置的欢迎语
	err = weCom.G_WeComApp.SendAddCustomerWelcomeMsg(context, contactWay, msg)
	if err != nil {
		return err
	}

	return err
}

// -------------------------------------------------------------------------------

func (srv *EmployeeService) HandleEditCustomer(context *gin.Context, event contract.EventInterface) (err error) {
	msg := &modelWecomEvent.EventExternalUserEdit{}
	err = event.ReadMessage(msg)
	if err != nil {
		return err
	}

	logger.Logger.Info("Handle Edit External Contact", zap.Any("msg", msg))

	// --------------------------------------------------
	// 从微信平台上获取客户信息
	rs, err := weCom.G_WeComApp.App.ExternalContact.Get(msg.ExternalUserID, "")
	if err != nil {
		return err
	}

	// 修改客户信息
	serviceCustomer := NewCustomerService(context)
	customer := serviceCustomer.NewCustomerFromWXContact(rs.ExternalContact)
	err = serviceCustomer.UpsertCustomerByWXCustomer(global.G_DBConnection, customer.WXCustomer)
	if err != nil {
		return err
	}

	// 从Event中的UserID获取员工
	employee, err := srv.GetEmployeeByUserID(global.G_DBConnection, msg.UserID)
	if err != nil {
		return err
	}

	// 保存操作日志
	_ = (&databasePowerLib.PowerOperationLog{}).SaveOps(global.G_DBConnection, employee.Name, employee,
		MODULE_CUSTOMER, "员工修改外部联系人", databasePowerLib.OPERATION_EVENT_UPDATE,
		customer.Name, customer, databasePowerLib.OPERATION_RESULT_SUCCESS)

	if len(rs.FollowUsers) > 0 {
		for _, followInfo := range rs.FollowUsers {
			// 修正客户与员工之间的关系
			pivot, err := (&models.RCustomerToEmployee{}).UpsertPivotByFollowUser(global.G_DBConnection, customer, followInfo)
			if err != nil {
				fmt.Dump(err.Error())
				continue
			}

			// 同步微信标签给客户
			if len(followInfo.Tags) > 0 {
				serviceWXTag := weCom.NewWXTagService(nil)
				err = serviceWXTag.SyncWXTagsByFollowInfos(global.G_DBConnection, pivot, followInfo)
			}
		}
	}

	return err
}
func (srv *EmployeeService) HandleAddHalfCustomer(context *gin.Context, event contract.EventInterface) (err error) {

	msg := &modelWecomEvent.EventExternalUserAddHalf{}
	err = event.ReadMessage(&msg)
	if err != nil {
		return err
	}

	logger.Logger.Info("Handle Add Half External Contact", zap.Any("msg", msg))

	// 加载客户联系
	serviceContactWay := NewContactWayService(nil)
	contactWay, err := serviceContactWay.GetContactWayByState(global.G_DBConnection, msg.State)
	if err != nil {
		return err
	}
	if *contactWay.WXContactWay.SkipVerify {
		err = srv.HandleAddCustomer(context, event)
	}

	return err
}
func (srv *EmployeeService) HandleDelCustomer(context *gin.Context, event contract.EventInterface) (err error) {

	msg := &modelWecomEvent.EventExternalUserDel{}
	err = event.ReadMessage(&msg)
	if err != nil {
		return err
	}

	logger.Logger.Info("Handle Del External Contact", zap.Any("msg", msg))

	// 解绑客户与员工关系
	customer, employee, err := srv.UnbindCustomerToEmployee(msg.ExternalUserID, msg.UserID)
	if err != nil {
		weCom.G_WeComApp.App.Logger.Error(err.Error())
		return err
	}
	// 保存操作日志
	_ = (&databasePowerLib.PowerOperationLog{}).SaveOps(global.G_DBConnection, employee.Name, employee,
		MODULE_CUSTOMER, "员工删除外部联系人", databasePowerLib.OPERATION_EVENT_DELETE,
		customer.Name, customer, databasePowerLib.OPERATION_RESULT_SUCCESS)

	//fmt.Dump(msg)

	return err

}
func (srv *EmployeeService) HandleDelFollowEmployee(context *gin.Context, event contract.EventInterface) (msg *modelWecomEvent.EventExternalUserDelFollowUser, err error) {
	msg = &modelWecomEvent.EventExternalUserDelFollowUser{}
	err = event.ReadMessage(&msg)
	if err != nil {
		return msg, err
	}
	logger.Logger.Info("Handle Del Follow User", zap.Any("msg", msg))

	// 解绑客户与员工关系
	customer, employee, err := srv.UnbindCustomerToEmployee(msg.ExternalUserID, msg.UserID)
	if err != nil {
		weCom.G_WeComApp.App.Logger.Error(err.Error())
		return msg, err
	}
	// 保存操作日志
	_ = (&databasePowerLib.PowerOperationLog{}).SaveOps(global.G_DBConnection, customer.Name, customer,
		MODULE_CUSTOMER, "外部联系人删除员工", databasePowerLib.OPERATION_EVENT_DELETE,
		employee.Name, employee, databasePowerLib.OPERATION_RESULT_SUCCESS)
	//fmt.Dump(msg)

	return msg, err

}
func (srv *EmployeeService) HandleTransferFail(context *gin.Context, event contract.EventInterface) (err error) {
	msg := &modelWecomEvent.EventExternalTransferFail{}
	err = event.ReadMessage(&msg)
	if err != nil {
		return err
	}
	logger.Logger.Info("Handle Transfer Fail", zap.Any("msg", msg))

	// 获取重分配的客户
	serviceCustomer := NewCustomerService(context)
	customer, err := serviceCustomer.GetCustomerByExternalUserID(global.G_DBConnection, msg.ExternalUserID)
	if err != nil {
		return err
	}
	// 获取重分配的员工
	employee, err := srv.GetEmployeeByUserID(global.G_DBConnection, msg.UserID)
	if err != nil {
		return err
	}

	// 保存操作记录
	_ = (&databasePowerLib.PowerOperationLog{}).SaveOps(global.G_DBConnection, customer.Name, customer,
		MODULE_CUSTOMER, "客户重新绑定员工失败", databasePowerLib.OPERATION_EVENT_UPDATE,
		employee.WXName, employee, databasePowerLib.OPERATION_RESULT_FAILED)

	return err
}

// --------------------------------------------------
// handle events from WeComApp Contact
// --------------------------------------------------

func (srv *EmployeeService) HandleEmployeeCreate(context *gin.Context, event contract.EventInterface) (err error) {

	msg := &modelWecomEvent.EventUserCreate{}
	err = event.ReadMessage(msg)
	if err != nil {
		return err
	}

	serviceWeComEmployee := weCom.NewWeComEmployeeService(nil)
	newEmployee := models.NewEmployee(object.NewCollection(&object.HashMap{
		"userID": msg.UserID,
	}))
	newEmployee.WXEmployee.WXDepartment = msg.Department

	err = serviceWeComEmployee.UpsertEmployees(global.G_DBConnection, []*models.Employee{
		newEmployee,
	},
		[]string{"wx_user_id", "wx_department"},
	)

	logger.Logger.Info("Handle Create Employee", zap.Any("msg", msg))

	return err
}

func (srv *EmployeeService) HandleEmployeeUpdate(context *gin.Context, event contract.EventInterface) (err error) {

	msg := &modelWecomEvent.EventUserUpdate{}
	err = event.ReadMessage(msg)
	if err != nil {
		return err
	}

	logger.Logger.Info("Handle Update Employee", zap.Any("msg", msg))

	return err
}

func (srv *EmployeeService) HandleEmployeeDelete(context *gin.Context, event contract.EventInterface) (err error) {

	msg := &modelWecomEvent.EventUserDelete{}
	err = event.ReadMessage(msg)
	if err != nil {
		return err
	}

	logger.Logger.Info("Handle Delete Employee", zap.Any("msg", msg))

	employee, err := srv.GetEmployeeByUserID(global.G_DBConnection, msg.UserID)

	err = global.G_DBConnection.Transaction(func(tx *gorm.DB) error {
		// 解绑员工和当前客户的关系
		err = (&models.RCustomerToEmployee{}).ClearPivotsByEmployeeID(tx, msg.UserID)
		if err != nil {
			return err
		}

		// 删除员工
		err = srv.DeleteEmployee(tx, employee)
		if err != nil {
			return err
		}

		return nil
	})

	// 保存操作记录
	_ = (&databasePowerLib.PowerOperationLog{}).SaveOps(global.G_DBConnection, "system", nil,
		MODULE_EMPLOYEE, "删除员工记录", databasePowerLib.OPERATION_EVENT_DELETE,
		employee.WXName, employee, databasePowerLib.OPERATION_RESULT_SUCCESS)

	return err
}

// --------------------------------------------------
// handle events from WeComApp contact Tag
// --------------------------------------------------

func (srv *EmployeeService) HandleContactTagUpdate(context *gin.Context, event contract.EventInterface) (err error) {

	msg := &modelWecomEvent.EventExternalUserAdd{}
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
	_, err = serviceCustomer.FollowEmployee(global.G_DBConnection, customer, followInfo)
	if err != nil {
		return pivot, err
	}

	return pivot, err

}

func (srv *EmployeeService) UnbindCustomerToEmployee(UserExternalID string, UserID string) (customer *models.Customer, employee *models.Employee, err error) {

	serviceWXTag := weCom.NewWXTagService(nil)
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

	// 删除客户和员工关系的微信标签
	err = serviceWXTag.ClearObjectWXTags(global.G_DBConnection, pivot)
	if err != nil {
		return customer, employee, err
	}

	// 解除客户和员工的关系
	err = serviceCustomer.UnfollowEmployees(global.G_DBConnection, customer, []*models.Employee{employee})
	if err != nil {
		return customer, employee, err
	}

	return customer, employee, err

}

func (srv *EmployeeService) SetRoot(db *gorm.DB, root *models.Employee) (err error) {

	return
}

func (srv *EmployeeService) GetRoot(db *gorm.DB) (root *models.Employee, err error) {
	root = &models.Employee{}
	tbEmployee := root.GetTableName(true)
	tbRoles := (&models2.Role{}).GetTableName(true)
	db = db.Model(root).
		//Debug().
		Joins("LEFT JOIN "+tbRoles+" AS tbRole ON tbRole.index_role_id = "+tbEmployee+".role_id").
		Where("tbRole.name = ?", models2.ROLE_SUPER_ADMIN_NAME)

	result := db.First(root)
	err = result.Error
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return root, err
}

func (srv *EmployeeService) GetRootRoleID(db *gorm.DB) (id string, err error) {

	role := &models2.Role{}
	db = db.Model(role).
		//Debug().
		Where("name = ?", models2.ROLE_SUPER_ADMIN_NAME)

	result := db.First(role)
	err = result.Error
	if err != nil {
		return id, err
	}

	id = role.UniqueID

	return id, err
}
