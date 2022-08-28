package role

import (
	modelsPowerLib "github.com/ArtisanCloud/PowerLibs/v2/authorization/rbac/models"
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/database/global"
	"github.com/gin-gonic/gin"
)

type ParaBindEmployeesToRoles struct {
	RoleID      string   `form:"roleID" json:"roleID" binding:"required"`
	EmployeeIDs []string `form:"employeeIDs" json:"employeeIDs" binding:"required,min=1"`
}

func ValidateBindRoleToEmployees(context *gin.Context) {
	var form ParaBindEmployeesToRoles

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	apiResponse := http.NewAPIResponse(context)
	role, err := convertParaToBindRoleToEmployees(&form)
	if err != nil {
		apiResponse.SetCode(config.API_ERR_CODE_REQUEST_PARAM_ERROR, config.API_RETURN_CODE_ERROR, "", err.Error()).ThrowJSONResponse(context)
	}
	context.Set("role", role)
	context.Set("employeeIDs", form.EmployeeIDs)
	context.Next()
}

func convertParaToBindRoleToEmployees(form *ParaBindEmployeesToRoles) (role *modelsPowerLib.Role, err error) {

	serviceRole := service.NewRoleService(nil)
	role, err = serviceRole.GetRoleByID(global.G_DBConnection, form.RoleID)
	if err != nil {
		return role, err
	}

	return role, err
}
