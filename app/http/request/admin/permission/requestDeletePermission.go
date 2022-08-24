package permission

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaDeletePermission struct {
	PermissionIDs []string `form:"permissionIDs" json:"permissionIDs" binding:"required,min=1"`
}

func ValidateDeletePermission(context *gin.Context) {
	var form ParaDeletePermission

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("permissionIDs", form.PermissionIDs)
	context.Next()
}
