package permission

import (
	"errors"
	"github.com/ArtisanCloud/PowerLibs/v2/authorization/rbac/models"
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/service"
	globalConfig "github.com/ArtisanCloud/PowerX/configs/global"
	"github.com/ArtisanCloud/PowerX/database/global"
	"github.com/gin-gonic/gin"
	"time"
)

type ParaUpdatePermission struct {
	PermissionID string  `form:"permissionID" json:"permissionID" binding:"required"`
	ObjectAlias  *string `form:"objectAlias" json:"objectAlias"`
	Description  *string `form:"description" json:"description"`
	ModuleID     *string `form:"moduleID" json:"moduleID"`
}

func ValidateUpdatePermission(context *gin.Context) {
	var form ParaUpdatePermission

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}
	apiResponse := http.NewAPIResponse(context)

	permission, err := convertParaToPermissionForUpdate(&form)
	if err != nil {
		apiResponse.SetCode(globalConfig.API_ERR_CODE_REQUEST_PARAM_ERROR, globalConfig.API_RETURN_CODE_ERROR, "", err.Error()).ThrowJSONResponse(context)
		return
	}

	context.Set("permission", permission)
	context.Next()
}

func convertParaToPermissionForUpdate(form *ParaUpdatePermission) (permission *models.Permission, err error) {

	serviceRBAC := service.NewRBACService(nil)
	permission, err = serviceRBAC.GetPermissionByID(global.G_DBConnection, form.PermissionID)
	if err != nil {
		return permission, err
	}
	if permission == nil {
		return permission, errors.New("permission not found")
	}

	permission.ObjectAlias = form.ObjectAlias
	permission.Description = form.Description
	permission.ModuleID = form.ModuleID
	permission.UpdatedAt = time.Now()

	err = permission.CheckPermissionNameAvailable(global.G_DBConnection)
	if err != nil {
		return nil, err
	}

	return permission, err
}
