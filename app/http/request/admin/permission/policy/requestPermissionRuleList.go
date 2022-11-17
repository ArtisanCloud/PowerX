package policy

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaPolicyList struct {
	//ParentID *string `form:"permissionID" json:"permissionID"`
	RoleID *string `form:"roleID" json:"roleID"`
}

func ValidatePolicyList(context *gin.Context) {
	var form ParaPolicyList

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}
	context.Set("params", &form)
	context.Next()
}
