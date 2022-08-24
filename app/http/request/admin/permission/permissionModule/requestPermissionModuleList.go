package permissionModule

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaPermissionModuleList struct {
	//ParentID *string `form:"permissionID" json:"permissionID"`
}

func ValidatePermissionModuleList(context *gin.Context) {
	var form ParaPermissionModuleList

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}
	context.Set("params", &form)
	context.Next()
}
