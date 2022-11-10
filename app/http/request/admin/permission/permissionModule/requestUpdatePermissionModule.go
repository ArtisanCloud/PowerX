package permissionModule

import (
	"errors"
	"github.com/ArtisanCloud/PowerLibs/v2/authorization/rbac/models"
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/service"
	globalConfig "github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/database/global"
	"github.com/gin-gonic/gin"
)

type ParaUpdatePermissionModule struct {
	PermissionModuleID string  `form:"permissionModuleID" json:"permissionModuleID" binding:"required"`
	Name               string  `form:"name" json:"name" binding:"required"`
	URI                string  `form:"uri" json:"uri"`
	Component          string  `form:"component" json:"component"`
	Icon               string  `form:"icon" json:"icon"`
	Description        string  `form:"description" json:"description"`
	ParentID           *string `form:"parentID" json:"parentID"`
}

func ValidateUpdatePermissionModule(context *gin.Context) {
	var form ParaUpdatePermissionModule

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	permissionModule, err := convertParaToPermissionModuleForUpdate(&form)
	if err != nil {
		apiResponse := http.NewAPIResponse(context)
		apiResponse.SetCode(globalConfig.API_ERR_CODE_REQUEST_PARAM_ERROR, globalConfig.API_RETURN_CODE_ERROR, "", err.Error()).ThrowJSONResponse(context)
		return
	}
	context.Set("permissionModule", permissionModule)
	context.Next()
}

func convertParaToPermissionModuleForUpdate(form *ParaUpdatePermissionModule) (permissionModule *models.PermissionModule, err error) {

	serviceRBAC := service.NewRBACService(nil)
	permissionModule, err = serviceRBAC.GetPermissionModuleByID(global.G_DBConnection, form.PermissionModuleID)
	if err != nil {
		return nil, err
	}
	if permissionModule == nil {
		return nil, errors.New("permission module not found")
	}

	permissionModule.Name = form.Name
	permissionModule.URI = form.URI
	permissionModule.Component = form.Component
	permissionModule.Icon = form.Icon
	permissionModule.Description = form.Description
	permissionModule.ParentID = form.ParentID

	err = permissionModule.CheckPermissionModuleNameAvailable(global.G_DBConnection)
	if err != nil {
		return nil, err
	}

	return permissionModule, err
}
