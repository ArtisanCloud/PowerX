package permissionModule

import (
	"github.com/ArtisanCloud/PowerLibs/v2/authorization/rbac/models"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	globalConfig "github.com/ArtisanCloud/PowerX/configs/global"
	"github.com/gin-gonic/gin"
)

type ParaInsertPermissionModule struct {
	Name        string `form:"name" json:"name" binding:"required"`
	Description string `form:"description" json:"description"`
	ParentID    string `form:"parentID" json:"parentID"`
}

func ValidateInsertPermissionModule(context *gin.Context) {
	var form ParaInsertPermissionModule

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	permissionModule, err := convertParaToPermissionModuleForInsert(&form)
	if err != nil {
		apiResponse := http.NewAPIResponse(context)
		apiResponse.SetCode(globalConfig.API_ERR_CODE_REQUEST_PARAM_ERROR, globalConfig.API_RETURN_CODE_ERROR, "", err.Error()).ThrowJSONResponse(context)
		return
	}
	context.Set("permissionModule", permissionModule)
	context.Next()
}

func convertParaToPermissionModuleForInsert(form *ParaInsertPermissionModule) (permissionModule *models.PermissionModule, err error) {

	permissionModule = models.NewPermissionModule(object.NewCollection(&object.HashMap{
		"name":        form.Name,
		"description": form.Description,
		"parentID":    form.ParentID,
	}))

	return permissionModule, err
}
