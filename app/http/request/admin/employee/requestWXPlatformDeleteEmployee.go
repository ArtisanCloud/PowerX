package employee

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaWXPlatformDeleteEmployee struct {
	UserID string `form:"userID" json:"userID" xml:"userID" binding:"required"`
}

func ValidateWXPlatformDeleteEmployee(context *gin.Context) {
	var form ParaWXPlatformDeleteEmployee

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("userID", form.UserID)
	context.Next()
}
