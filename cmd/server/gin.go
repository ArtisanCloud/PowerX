package server

import (
	"github.com/ArtisanCloud/PowerX/boostrap"
	"github.com/ArtisanCloud/PowerX/config"
	logger "github.com/ArtisanCloud/PowerX/loggerManager"
	"github.com/ArtisanCloud/PowerX/routes"
	"github.com/ArtisanCloud/PowerX/routes/global"
	"github.com/spf13/cobra"
)

func LaunchServer(cmd *cobra.Command, args []string) {

	var err error

	err = boostrap.InitConfig()
	if err != nil {
		panic(err)
		return
	}

	// 模拟系统已经安装成功
	if config.G_AppConfigure.SystemConfig.Installed {
		// init project
		err = boostrap.InitProject()
		if err != nil {
			logger.Logger.Error("InitProject error:", err)
			panic(err)
			return
		}
	}

	// Initialize the routes
	err = routes.InitializeRoutes()
	if err != nil {
		logger.Logger.Error("config router apis error:", err)
		panic(err)
		return
	}

	// Start serving the application
	// listen and serve on 0.0.0.0:8080 (for windows "localhost:8080®")
	err = global.Router.Run(config.G_AppConfigure.ServerConfig.Host + ":" + config.G_AppConfigure.ServerConfig.Port)
	if err != nil {
		logger.Logger.Error("run router error:", err)
		panic(err)
		return
	}

}
