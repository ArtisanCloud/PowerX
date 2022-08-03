package main

import (
	"github.com/ArtisanCloud/PowerX/boostrap"
	"github.com/ArtisanCloud/PowerX/config"
	logger "github.com/ArtisanCloud/PowerX/loggerManager"
)
import _ "github.com/ArtisanCloud/PowerX/config"
import "github.com/ArtisanCloud/PowerX/routes"

import (
	"github.com/gin-gonic/gin"
)

var Router *gin.Engine

func main() {

	var err error
	// init project
	err = boostrap.InitProject()
	if err != nil {
		logger.Logger.Error("InitProject error:", err)
		panic(err)
		return
	}

	// Router the router as the default one provided by Gin
	Router = gin.Default()
	if Router == nil {
		logger.Logger.Error("init router failed")
		panic(err)
		return
	}

	// Initialize the routes
	err = routes.InitializeRoutes(Router)
	if err != nil {
		logger.Logger.Error("config router apis error:", err)
		panic(err)
		return
	}

	// Start serving the application
	// listen and serve on 0.0.0.0:8080 (for windows "localhost:8080®")
	err = Router.Run(config.AppConfigure.Server.Host + ":" + config.AppConfigure.Server.Port)
	if err != nil {
		logger.Logger.Error("run router error:", err)
		panic(err)
		return
	}

}
