package routes

import (
	"github.com/ArtisanCloud/PowerX/app/http/controllers/web"
	"github.com/ArtisanCloud/PowerX/app/http/middleware"
	"github.com/ArtisanCloud/PowerX/routes/global"
	"github.com/gin-gonic/gin"
	"net/http"
)

func InitializeWebRoutes() {

	apiRouter := global.Router.Group("/")
	{
		apiRouter.Use(middleware.Maintenance)
		{
			// wx
			apiRouter.GET("/", web.WebGetHome)

			apiRouter.GET("/WW_verify_UTeyopi6l6j9FVgK.txt", func(ctx *gin.Context) {
				ctx.String(http.StatusOK, "UTeyopi6l6j9FVgK")
			})

			apiRouter.GET("/pay.html", func(ctx *gin.Context) {
				ctx.HTML(http.StatusOK, "index.html", gin.H{})
			})

		}
	}

}
