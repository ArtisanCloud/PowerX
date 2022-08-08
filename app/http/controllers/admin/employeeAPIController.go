package admin

import (
	database2 "github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	models2 "github.com/ArtisanCloud/PowerSocialite/v2/src/models"
	"github.com/ArtisanCloud/PowerX/app/http/controllers/api"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/app/service/wx/wecom"
	"github.com/ArtisanCloud/PowerX/boostrap/global"
	"github.com/ArtisanCloud/PowerX/config"
	logger "github.com/ArtisanCloud/PowerX/loggerManager"
	"github.com/gin-gonic/gin"
)

type EmployeeAPIController struct {
	*api.APIController
	ServiceEmployee *service.EmployeeService
}

func NewEmployeeAPIController(context *gin.Context) (ctl *EmployeeAPIController) {

	return &EmployeeAPIController{
		APIController:   api.NewAPIController(context),
		ServiceEmployee: service.NewEmployeeService(context),
	}
}

func APISyncWXEmployees(context *gin.Context) {
	ctl := NewEmployeeAPIController(context)

	defer api.RecoverResponse(context, "wx.api.employee.sync")

	var err error
	// sync departments
	serviceDepartment := service.NewDepartmentService(context)
	err = serviceDepartment.SyncDepartments()
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_UPSERT_DEPARTMENT, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	// sync employees
	err = ctl.ServiceEmployee.SyncEmployees()
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_UPSERT_EMPLOYEE, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, err)

}

func APISyncEmployeeAndWXAccount(context *gin.Context) {
	ctl := NewEmployeeAPIController(context)

	defer api.RecoverResponse(context, "wx.api.customer.sync")

	var err error
	// sync employees
	err = ctl.ServiceEmployee.SyncEmployees()
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_UPSERT_EMPLOYEE, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	// sync accounts
	customerService := service.NewCustomerService(context)
	err = customerService.SyncCustomers(nil, "")
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_UPSERT_ACCOUNT, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, err)

}

func APIGetEmployeeList(context *gin.Context) {
	ctl := NewEmployeeAPIController(context)

	params, _ := context.Get("params")
	para := params.(request.ParaList)

	defer api.RecoverResponse(context, "api.admin.employee.list")

	arrayList, err := ctl.ServiceEmployee.GetList(global.DBConnection, nil, para.Page, para.PageSize)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_EMPLOYEE_LIST, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, arrayList)
}

func APIGetEmployeeDetail(context *gin.Context) {
	ctl := NewEmployeeAPIController(context)

	userIDInterface, _ := context.Get("userID")
	userID := userIDInterface.(string)

	defer api.RecoverResponse(context, "api.admin.employee.detail")

	employee, err := ctl.ServiceEmployee.GetEmployeeByUserID(global.DBConnection, userID)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_EMPLOYEE_DETAIL, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, employee)
}

func APIUpsertEmployee(context *gin.Context) {
	ctl := NewEmployeeAPIController(context)

	params, _ := context.Get("employee")
	employee := params.(*models.Employee)

	defer api.RecoverResponse(context, "api.admin.employee.upsert")

	var err error
	employee, err = ctl.ServiceEmployee.UpsertEmployee(global.DBConnection, employee)

	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_UPSERT_EMPLOYEE, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, employee)

}

func APIDeleteEmployees(context *gin.Context) {
	ctl := NewEmployeeAPIController(context)

	uuids, _ := context.Get("uuids")

	defer api.RecoverResponse(context, "api.admin.employee.delete")

	employees, err := ctl.ServiceEmployee.GetEmployees(global.DBConnection, uuids.([]string))
	if len(employees) <= 0 {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_EMPLOYEE_LIST, config.API_RETURN_CODE_ERROR, "", "")
		panic(ctl.RS)
		return
	}

	err = ctl.ServiceEmployee.DeleteEmployees(global.DBConnection, employees)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_DELETE_EMPLOYEE, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, err)
}

func APIBindCustomerToEmployee(context *gin.Context) {
	ctl := NewEmployeeAPIController(context)

	customerInterface, _ := context.Get("customer")
	customer := customerInterface.(*models.Customer)
	employeeInterface, _ := context.Get("employee")
	employee := employeeInterface.(*models.Employee)
	followInfoInterface, _ := context.Get("followInfo")
	followInfo := followInfoInterface.(*models2.FollowUser)

	defer api.RecoverResponse(context, "api.admin.employee.bind.customer")

	pivot, err := ctl.ServiceEmployee.BindCustomerToEmployee(customer.ExternalUserID.String, followInfo)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_BIND_CUSOTMER_TO_EMPLOYEE, config.API_RETURN_CODE_ERROR, "", "")
		panic(ctl.RS)
		return
	}
	// save operation log
	_ = (&database2.PowerOperationLog{}).SaveOps(global.DBConnection, customer.Name, customer,
		service.MODULE_CUSTOMER, "系统绑定外部联系人与员工", database2.OPERATION_EVENT_CREATE,
		employee.Name, employee, database2.OPERATION_RESULT_SUCCESS)

	if len(followInfo.Tags) > 0 {
		serviceWXTag := wecom.NewWXTagService(nil)
		err = serviceWXTag.SyncWXTagsByFollowInfos(global.DBConnection, pivot, followInfo)
		if err != nil {
			ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_SYNC_WX_TAG, config.API_RETURN_CODE_ERROR, "", "")
			panic(ctl.RS)
			return
		}
	}

	ctl.RS.Success(context, err)
}

func APIUnbindCustomerToEmployee(context *gin.Context) {
	ctl := NewEmployeeAPIController(context)

	customerInterface, _ := context.Get("customer")
	customer := customerInterface.(*models.Customer)
	employeeInterface, _ := context.Get("employee")
	employee := employeeInterface.(*models.Employee)

	defer api.RecoverResponse(context, "api.admin.employee.bind.customer")

	_, _, err := ctl.ServiceEmployee.UnbindCustomerToEmployee(customer.ExternalUserID.String, employee.WXUserID.String)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_BIND_CUSOTMER_TO_EMPLOYEE, config.API_RETURN_CODE_ERROR, "", "")
		panic(ctl.RS)
		return
	}
	// save operation log
	_ = (&database2.PowerOperationLog{}).SaveOps(global.DBConnection, customer.Name, customer,
		service.MODULE_CUSTOMER, "系统解绑外部联系人与员工", database2.OPERATION_EVENT_DELETE,
		employee.Name, employee, database2.OPERATION_RESULT_SUCCESS)

	ctl.RS.Success(context, err)
}

func APIEmployeeBindSyncedWXDepartments(context *gin.Context) {
	ctl := NewEmployeeAPIController(context)

	defer api.RecoverResponse(context, "api.admin.employee.bind.syncedWXDepartments")

	employees, err := ctl.ServiceEmployee.GetAllEmployees(global.DBConnection, nil)
	if err != nil {
		panic(ctl.RS)
		return
	}

	for _, employee := range employees {
		if employee.WXEmployee.WXDepartments != "" {
			departmentIDs := []int{}
			err = object.JsonDecode([]byte(employee.WXEmployee.WXDepartments), &departmentIDs)
			if err != nil {
				logger.Logger.Error(err.Error())
				continue
			}

			err = ctl.ServiceEmployee.SyncDepartmentIDsToEmployee(global.DBConnection, employee, departmentIDs)
			if err != nil {
				logger.Logger.Error(err.Error())
				continue
			}
		}
	}

	ctl.RS.Success(context, err)
}

// ------------------------------------------------------------

func APIGetEmployeeListOnWXPlatform(context *gin.Context) {
	ctl := NewEmployeeAPIController(context)

	departmentIDInterface, _ := context.Get("departmentID")
	departmentID := departmentIDInterface.(int)

	defer api.RecoverResponse(context, "api.admin.employee.list")

	arrayList, err := wecom.WeComEmployee.App.User.GetDepartmentUsers(departmentID, 1)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_EMPLOYEE_LIST, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, arrayList)
}

func APIGetEmployeeDetailOnWXPlatform(context *gin.Context) {
	ctl := NewEmployeeAPIController(context)

	userIDInterface, _ := context.Get("userID")
	userID := userIDInterface.(string)

	defer api.RecoverResponse(context, "api.admin.employee.detail")

	result, err := wecom.WeComEmployee.App.User.Get(userID)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_EMPLOYEE_DETAIL, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, result)
}

func APIDeleteEmployeesOnWXPlatform(context *gin.Context) {
	ctl := NewEmployeeAPIController(context)

	userIDInterface, _ := context.Get("userID")
	userID := userIDInterface.(string)

	defer api.RecoverResponse(context, "api.admin.employee.delete")

	result, err := wecom.WeComEmployee.App.User.Delete(userID)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_DELETE_EMPLOYEE, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, result)
}
