package permissionModule

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaPermissionModuleDetail struct {
	PermissionModuleID string `form:"permissionModuleID" json:"permissionModuleID" binding:"required"`
}

func ValidatePermissionModuleDetail(context *gin.Context) {
	var form ParaPermissionModuleDetail

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}
	context.Set("permissionModuleID", form.PermissionModuleID)
	context.Next()
}
