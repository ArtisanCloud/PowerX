package wx

import (
	"github.com/ArtisanCloud/PowerX/app/http/controllers/api"
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/app/service/wx/wecom"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/database"
	"github.com/gin-gonic/gin"
)

type DepartmentAPIController struct {
	*api.APIController
	ServiceDepartment *service.DepartmentService
}

func NewDepartmentAPIController(context *gin.Context) (ctl *DepartmentAPIController) {

	return &DepartmentAPIController{
		APIController:     api.NewAPIController(context),
		ServiceDepartment: service.NewDepartmentService(context),
	}
}

func APISyncWXDepartments(context *gin.Context) {
	ctl := NewDepartmentAPIController(context)

	defer api.RecoverResponse(context, "wecom.api.department.sync")

	var err error

	err = ctl.ServiceDepartment.SyncDepartments()
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_UPSERT_DEPARTMENT, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, err)

}

func APIGetDepartmentList(context *gin.Context) {
	ctl := NewDepartmentAPIController(context)

	defer api.RecoverResponse(context, "api.admin.department.list")

	arrayList, err := ctl.ServiceDepartment.GetDepartments(database.DBConnection)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_DEPARTMENT_LIST, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, arrayList)
}

func APIGetDepartmentDetail(context *gin.Context) {
	ctl := NewDepartmentAPIController(context)

	departmentIDInterface, _ := context.Get("departmentID")
	departmentID := departmentIDInterface.(*int)

	defer api.RecoverResponse(context, "api.admin.department.detail")

	department, err := ctl.ServiceDepartment.GetDepartmentsByIDs(database.DBConnection, []int{*departmentID})
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_DEPARTMENT_DETAIL, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, department)
}

// ------------------------------------------------------------

func APIGetDepartmentSimpleListOnWXPlatform(context *gin.Context) {
	ctl := NewDepartmentAPIController(context)

	departmentIDInterface, _ := context.Get("departmentID")
	departmentID := departmentIDInterface.(*int)

	defer api.RecoverResponse(context, "api.admin.department.list")

	arrayList, err := wecom.WeComEmployee.App.Department.SimpleList(*departmentID)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_DEPARTMENT_LIST, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, arrayList)
}

func APIGetDepartmentListOnWXPlatform(context *gin.Context) {
	ctl := NewDepartmentAPIController(context)

	departmentIDInterface, _ := context.Get("departmentID")
	departmentID := departmentIDInterface.(*int)

	defer api.RecoverResponse(context, "api.admin.department.detail")

	result, err := wecom.WeComEmployee.App.Department.List(*departmentID)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_DEPARTMENT_DETAIL, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, result)
}
