package menu

import "github.com/gin-gonic/gin"

func RegisterAPIRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup) {
	menuGroup := protectedGroup.Group("/menus")
	{
		menuGroup.GET("/", AdminMenusHandler)
	}
}
