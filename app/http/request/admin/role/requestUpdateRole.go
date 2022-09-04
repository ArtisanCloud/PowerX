package role

import (
	"errors"
	"github.com/ArtisanCloud/PowerLibs/v2/authorization/rbac/models"
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/database/global"
	"github.com/gin-gonic/gin"
	"time"
)

type ParaUpdateRole struct {
	RoleID string `form:"roleID" json:"roleID" binding:"required"`
	Name   string `form:"name" json:"name" binding:"required"`
}

func ValidateUpdateRole(context *gin.Context) {
	var form ParaUpdateRole

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}
	apiResponse := http.NewAPIResponse(context)

	role, err := convertParaToRoleForUpdate(&form)
	if err != nil {
		apiResponse.SetCode(config.API_ERR_CODE_REQUEST_PARAM_ERROR, config.API_RETURN_CODE_ERROR, "", err.Error()).ThrowJSONResponse(context)
		return
	}

	context.Set("role", role)
	context.Next()
}

func convertParaToRoleForUpdate(form *ParaUpdateRole) (role *models.Role, err error) {

	serviceRole := service.NewRoleService(nil)
	role, err = serviceRole.GetRoleByID(global.G_DBConnection, form.RoleID)
	if err != nil {
		return role, err
	}
	if role == nil {
		return role, errors.New("role not found")
	}

	role.Name = form.Name
	role.UpdatedAt = time.Now()

	err = role.CheckRoleNameAvailable(global.G_DBConnection)
	if err != nil {
		return nil, err
	}

	return role, err
}
