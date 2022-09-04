package wx

import (
	"github.com/ArtisanCloud/PowerX/app/http/controllers/api"
	"github.com/ArtisanCloud/PowerX/app/service"
	global2 "github.com/ArtisanCloud/PowerX/app/service/wx/wecom"
	"github.com/ArtisanCloud/PowerX/config"
	globalDatabase "github.com/ArtisanCloud/PowerX/database/global"
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

	err = ctl.ServiceDepartment.SyncDepartments(0)
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

	arrayList, err := ctl.ServiceDepartment.GetDepartments(globalDatabase.G_DBConnection)
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

	department, err := ctl.ServiceDepartment.GetDepartmentsByIDs(globalDatabase.G_DBConnection, []int{*departmentID})
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

	arrayList, err := global2.G_WeComEmployee.App.Department.SimpleList(*departmentID)
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

	result, err := global2.G_WeComEmployee.App.Department.List(*departmentID)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_DEPARTMENT_DETAIL, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, result)
}
