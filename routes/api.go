package routes

import (
	apis "github.com/ArtisanCloud/PowerX/routes/api"
	"github.com/gin-gonic/gin"
)

func InitializeAPIRoutes(router *gin.Engine) {

	apis.InitAdminAPIRoutes(router)
	apis.InitWXRoutes(router)

}
