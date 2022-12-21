package routes

import (
	"github.com/ArtisanCloud/PowerX/app/http/middleware"
	"github.com/ArtisanCloud/PowerX/routes/global"
	"github.com/gin-gonic/gin"
	"net/http"
)

func InitializeWebRoutes() {

	// Serve frontend static files
	//global.G_Router.Use(static.Serve("/", static.LocalFile("resources/html/", true)))
	//global.G_Router.NoRoute(func(c *gin.Context) {
	//	c.Redirect(http.StatusMovedPermanently, "/")
	//})
	//global.G_Router.GET("/__umi_ping", rootAPI.APIPing)

	apiRouter := global.G_Router.Group("/")
	{
		apiRouter.Use(middleware.CheckInstalled, middleware.Maintenance)
		{
			// wechat
			//apiRouter.GET("/", web.WebGetHome)

			apiRouter.GET("/WW_verify_UTeyopi6l6j9FVgK.txt", func(ctx *gin.Context) {
				ctx.String(http.StatusOK, "UTeyopi6l6j9FVgK")
			})

			apiRouter.GET("/pay.html", func(ctx *gin.Context) {
				ctx.HTML(http.StatusOK, "index.html", gin.H{})
			})

		}
	}

}
