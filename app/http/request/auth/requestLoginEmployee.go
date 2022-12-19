package auth

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaLoginEmployee struct {
	Email    string `form:"email" json:"email"  binding:"required"`
	Password string `form:"password" json:"password"  binding:"required"`
}

func ValidateLoginEmployee(context *gin.Context) {
	var form ParaLoginEmployee

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("params", &form)
	context.Next()
}
