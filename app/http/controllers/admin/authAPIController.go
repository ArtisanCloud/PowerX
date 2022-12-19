package admin

import (
	"github.com/ArtisanCloud/PowerLibs/v2/helper"
	"github.com/ArtisanCloud/PowerX/app/http/controllers/api"
	"github.com/ArtisanCloud/PowerX/app/http/request/auth"
	"github.com/ArtisanCloud/PowerX/app/service"
	global2 "github.com/ArtisanCloud/PowerX/config"
	globalDatabase "github.com/ArtisanCloud/PowerX/database/global"
	"github.com/gin-gonic/gin"
)

type AuthAPIController struct {
	*api.APIController
	ServiceEmployee *service.EmployeeService
}

func NewAuthAPIController(context *gin.Context) (ctl *AuthAPIController) {

	return &AuthAPIController{
		APIController:   api.NewAPIController(context),
		ServiceEmployee: service.NewEmployeeService(context),
	}
}

func APILoginEmployee(context *gin.Context) {

	ctl := NewAuthAPIController(context)
	paramsInterface, _ := context.Get("params")
	param := paramsInterface.(*auth.ParaLoginEmployee)

	defer api.RecoverResponse(context, "api.admin.login.employee")

	employee, err := ctl.ServiceEmployee.GetEmployeeByEmail(globalDatabase.G_DBConnection, param.Email)
	if err != nil {
		ctl.RS.SetCode(global2.API_ERR_CODE_FAIL_TO_GET_EMPLOYEE_DETAIL, global2.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	isSamePassword, err := helper.CheckPassword(*employee.Password, param.Password)
	if err != nil {
		ctl.RS.SetCode(global2.API_ERR_CODE_INVALID_PASSWORD, global2.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}
	if !isSamePassword {
		ctl.RS.SetCode(global2.API_ERR_CODE_INVALID_PASSWORD, global2.API_RETURN_CODE_ERROR, "", "")
		panic(ctl.RS)
		return
	}

	serviceAuth := service.NewAuthService(context)
	strToken, result := serviceAuth.CreateTokenForEmployee(employee)
	if !result {
		ctl.RS.SetCode(global2.API_ERR_CODE_TOKEN_REVOKED, global2.API_RETURN_CODE_ERROR, "", "")
		ctl.RS.ThrowJSONResponse(context)
		return
	}
	res := map[string]interface{}{
		"token_type":    "Bearer",
		"expires_in":    service.InExpiredSecond,
		"access_token":  strToken,
		"refresh_token": "",
	}

	// 正常返回json
	ctl.RS.Success(context, res)

}
