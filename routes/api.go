package routes

import (
	apis "github.com/ArtisanCloud/PowerX/routes/api"
)

func InitializeAPIRoutes() {

	apis.InitAdminAPIRoutes()
	apis.InitWXRoutes()

}
