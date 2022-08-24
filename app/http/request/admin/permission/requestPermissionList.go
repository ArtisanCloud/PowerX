package permission

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaPermissionList struct {
	//ParentID *string `form:"permissionID" json:"permissionID"`
}

func ValidatePermissionList(context *gin.Context) {
	var form ParaPermissionList

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}
	context.Set("params", &form)
	context.Next()
}
