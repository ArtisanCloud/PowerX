package menu

import "github.com/gin-gonic/gin"

func RegisterAPIRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup) {
	menuGroup := protectedGroup.Group("/admin/menus")
	{
		// 用 "" 避免 Gin 对 "/admin/menus" -> "/admin/menus/" 的 301 重定向，减少前端代理/缓存场景下的 404 误判
		menuGroup.GET("", AdminMenusHandler)
	}
}
