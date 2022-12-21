package routes

import (
	logger "github.com/ArtisanCloud/PowerX/loggerManager"
	"github.com/ArtisanCloud/PowerX/routes/global"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

var Router *gin.Engine

func InitializeRoutes() (err error) {

	// Router the router as the default one provided by Gin
	global.G_Router = gin.Default()
	if global.G_Router == nil {
		logger.Logger.Error("init router failed")
		return
	}

	err = global.G_Router.SetTrustedProxies([]string{"127.0.0.1"})
	if err != nil {
		logger.Logger.Error("SetTrustedProxies error:", err)
		return err
	}
	global.G_Router.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders: []string{
			"Origin", "Content-Type", "Authorization",
			"x-requested-with", "Access-Control-Allow-Origin",
		},
		AllowCredentials: true,
		AllowOriginFunc: func(origin string) bool {
			logger.Logger.Info("origin: ", origin)
			return true
		},
	}))

	//global.G_Router.LoadHTMLGlob("resources/html/*")

	InitializeWebRoutes()
	InitializeAPIRoutes()

	return err
}
