package routes

import (
	apis "github.com/ArtisanCloud/PowerX/routes/api"
)

func InitializeAPIRoutes() {

	apis.InitRootAPIRoutes()
	apis.InitAdminAPIRoutes()
	apis.InitWXRoutes()

}
