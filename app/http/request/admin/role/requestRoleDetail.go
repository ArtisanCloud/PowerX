package role

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaRoleDetail struct {
	RoleID string `form:"roleID" json:"roleID" binding:"required"`
}

func ValidateRoleDetail(context *gin.Context) {
	var form ParaRoleDetail

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}
	context.Set("roleID", form.RoleID)
	context.Next()
}
