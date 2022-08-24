package permission

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaPermissionDetail struct {
	PermissionID string `form:"permissionID" json:"permissionID" binding:"required"`
}

func ValidatePermissionDetail(context *gin.Context) {
	var form ParaPermissionDetail

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}
	context.Set("permissionID", form.PermissionID)
	context.Next()
}
