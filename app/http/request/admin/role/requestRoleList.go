package role

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaRoleList struct {
	//RoleID *string `form:"roleID" json:"roleID" binding:"required"`
}

func ValidateRoleList(context *gin.Context) {
	var form ParaRoleList

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}
	context.Set("params", &form)
	context.Next()
}
