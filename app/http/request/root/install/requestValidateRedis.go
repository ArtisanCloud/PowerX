package root

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/gin-gonic/gin"
)

type ParaValidateRedis struct {
	Redis *config.RedisConfig `form:"redis" json:"redis"  binding:"required"`
}

func ValidateRedis(context *gin.Context) {
	var form ParaValidateRedis

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("params", &form)
	context.Next()
}
