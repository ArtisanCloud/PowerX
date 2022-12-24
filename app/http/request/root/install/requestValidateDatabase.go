package root

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/gin-gonic/gin"
)

type ParaValidateDatabase struct {
	Database *config.DatabaseConfig `form:"database" json:"database"  binding:"required"`
}

func ValidateDatabase(context *gin.Context) {
	var form ParaValidateDatabase

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("params", &form)
	context.Next()
}
