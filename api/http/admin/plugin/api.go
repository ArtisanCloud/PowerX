package plugin

import (
	"github.com/gin-gonic/gin"
)

func RegisterAPIRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup) {
	// 如果你已有 Admin 的中间件（鉴权/租户），可以在这里加到 Group 上
	grp := protectedGroup.Group("/admin/plugins")
	{
		grp.GET("/", PluginListHandler)       // GET  /api/v1/admin/plugins
		grp.GET("/menus", PluginMenusHandler) // GET  /api/v1/admin/plugins/menus

		grp.POST("/:id/enable", PluginEnableHandler)   // POST /api/v1/admin/plugins/:id/enable
		grp.POST("/:id/disable", PluginDisableHandler) // POST /api/v1/admin/plugins/:id/disable
		grp.POST("/:id/restart", PluginRestartHandler) // POST /api/v1/admin/plugins/:id/restart
		grp.GET("/:id/status", PluginStatusHandler)    // GET /api/v1/admin/plugins/:id/status

		grp.POST("/install/local", PluginInstallLocalHandler) // POST /api/v1/admin/plugins/:id/disable
		grp.POST("/install/url", PluginInstallURLHandler)     // POST /api/v1/admin/plugins/install/url

		grp.POST("/:id/switch_version", PluginSwitchVersionHandler) // POST /api/v1/admin/plugins/:id/switch_version
		grp.POST("/:id/uninstall", PluginUninstallHandler)          // POST /api/v1/admin/plugins/:id/uninstall

		grp.GET("/:id/logs", PluginLogsHandler) // GET /api/v1/admin/plugins/:id/logs

	}
}
