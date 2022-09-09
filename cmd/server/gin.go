package server

import (
	"errors"
	"github.com/ArtisanCloud/PowerLibs/v2/fmt"
	"github.com/ArtisanCloud/PowerX/boostrap"
	"github.com/ArtisanCloud/PowerX/config"
	logger "github.com/ArtisanCloud/PowerX/loggerManager"
	"github.com/ArtisanCloud/PowerX/routes"
	"github.com/ArtisanCloud/PowerX/routes/global"
	"github.com/spf13/cobra"
	"log"
	"net/http"
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

	// create http server
	address := config.G_AppConfigure.ServerConfig.Host + ":" + config.G_AppConfigure.ServerConfig.Port
	global.G_Server = &http.Server{
		Addr:    address,
		Handler: global.G_Router,
	}

	fmt.Dump(address)
	if err = global.G_Server.ListenAndServe(); err != nil && errors.Is(err, http.ErrServerClosed) {
		log.Printf("listen: %s\n", err)
		return
	}

}
