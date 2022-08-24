package permissionModule

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaDeletePermissionModule struct {
	PermissionModuleIDs []string `form:"permissionModuleIDs" json:"permissionModuleIDs" binding:"required,min=1"`
}

func ValidateDeletePermissionModule(context *gin.Context) {
	var form ParaDeletePermissionModule

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("permissionModuleIDs", form.PermissionModuleIDs)
	context.Next()
}
