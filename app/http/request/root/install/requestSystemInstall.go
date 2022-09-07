package root

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/gin-gonic/gin"
)

type ParaSystemInstall struct {
	AppConfig *config.AppConfig `form:"appConfig" json:"appConfig"  binding:"required"`
}

func ValidateSystemInstall(context *gin.Context) {
	var form ParaSystemInstall

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("params", &form)
	context.Next()
}
