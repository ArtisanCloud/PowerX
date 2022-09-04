package role

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaDeleteRole struct {
	RoleIDs []string `form:"roleIDs" json:"roleIDs" binding:"required,min=1"`
}

func ValidateDeleteRole(context *gin.Context) {
	var form ParaDeleteRole

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("roleIDs", form.RoleIDs)
	context.Next()
}
