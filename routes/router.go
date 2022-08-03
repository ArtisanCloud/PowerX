package routes

import (
	logger "github.com/ArtisanCloud/PowerX/loggerManager"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

var Router *gin.Engine

func InitializeRoutes(router *gin.Engine) (err error) {

	Router = router
	//Router.Use(UBT.GinEsLog(logger.UBTHandler))

	err = Router.SetTrustedProxies([]string{"127.0.0.1"})
	if err != nil {
		logger.Logger.Error("SetTrustedProxies error:", err)
		return err
	}

	Router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		AllowCredentials: true,
		AllowOriginFunc: func(origin string) bool {
			logger.Logger.Info("origin: ", origin)
			return true
		},
	}))

	Router.LoadHTMLGlob("resources/html/*")

	InitializeWebRoutes(router)
	InitializeAPIRoutes(router)

	return err
}
