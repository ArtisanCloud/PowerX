package permission

import (
	"github.com/ArtisanCloud/PowerLibs/v2/authorization/rbac/models"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	globalConfig "github.com/ArtisanCloud/PowerX/config/global"
	"github.com/gin-gonic/gin"
)

type ParaInsertPermission struct {
	ObjectAlias string `form:"objectAlias" json:"objectAlias"`
	ObjectValue string `form:"objectValue" json:"objectValue"`
	Action      string `form:"action" json:"action"`
	Description string `form:"description" json:"description"`
	ModuleID    string `form:"moduleID" json:"moduleID"`
}

func ValidateInsertPermission(context *gin.Context) {
	var form ParaInsertPermission

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	permission, err := convertParaToPermissionForInsert(&form)
	if err != nil {
		apiResponse := http.NewAPIResponse(context)
		apiResponse.SetCode(globalConfig.API_ERR_CODE_REQUEST_PARAM_ERROR, globalConfig.API_RETURN_CODE_ERROR, "", err.Error()).ThrowJSONResponse(context)
		return
	}
	context.Set("permission", permission)
	context.Next()
}

func convertParaToPermissionForInsert(form *ParaInsertPermission) (permission *models.Permission, err error) {

	permission = models.NewPermission(object.NewCollection(&object.HashMap{
		"objectAlias": form.ObjectAlias,
		"objectValue": form.ObjectValue,
		"action":      form.Action,
		"description": form.Description,
		"moduleID":    form.ModuleID,
	}))

	return permission, err
}
