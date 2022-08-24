package policy

import (
	"github.com/ArtisanCloud/PowerLibs/v2/authorization/rbac/models"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaUpdatePolicy struct {
	Policies []*models.RolePolicy `form:"policies" json:"policies" binding:"required,min=1"`
}

func ValidateUpdatePolicy(context *gin.Context) {
	var form ParaUpdatePolicy

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("policies", form.Policies)
	context.Next()
}
