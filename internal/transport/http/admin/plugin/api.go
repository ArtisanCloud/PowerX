package plugin

import (
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/gin-gonic/gin"
)

func RegisterAPIRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	cfg := config.GetGlobalConfig()
	SetMarketplaceBasePrefix(cfg.Plugin.BasePrefix)

	grp := protectedGroup.Group("/admin/plugins")
	// 如果你已有 Admin 的中间件（鉴权/租户），可以在这里加到 Group 上
	grp.Use(middleware.AdminOnlyMiddleware())
	{
		grp.GET("/marketplace/plugins", MarketplaceListHandler(cfg.Plugin.BasePrefix))
		grp.GET("/marketplace/plugins_v2", MarketplaceListV2Handler(cfg.Plugin.BasePrefix))

		grp.GET("/", PluginListHandler)       // GET  /api/v1/admin/plugins
		grp.GET("/:id", PluginGetHandler)     // GET  /api/v1/admin/plugins/:id
		grp.GET("/menus", PluginMenusHandler) // GET  /api/v1/admin/plugins/menus

		// 系统级：启停/重启/状态/安装/卸载/切换版本
		grp.POST("/:id/enable", PluginEnableHandler)   // POST /api/v1/admin/plugins/:id/enable
		grp.POST("/:id/disable", PluginDisableHandler) // POST /api/v1/admin/plugins/:id/disable
		grp.POST("/:id/restart", PluginRestartHandler) // POST /api/v1/admin/plugins/:id/restart
		grp.GET("/:id/status", PluginStatusHandler)    // GET /api/v1/admin/plugins/:id/status

		grp.POST("/install/local", PluginInstallLocalHandler) // POST /api/v1/admin/plugins/install/local
		grp.POST("/install/url", PluginInstallURLHandler)     // POST /api/v1/admin/plugins/install/url

		grp.POST("/:id/switch_version", PluginSwitchVersionHandler) // POST /api/v1/admin/plugins/:id/switch_version
		grp.POST("/:id/uninstall", PluginUninstallHandler)          // POST /api/v1/admin/plugins/:id/uninstall

		grp.GET("/:id/logs", PluginLogsHandler) // GET /api/v1/admin/plugins/:id/logs

		// 租户级：查询配置、启用/停用（仅影响本租户）、凭证查看与轮换
		grp.GET("/:id/tenant_config", PluginTenantConfigHandler(deps))
		grp.POST("/:id/tenant_enable", PluginTenantEnableHandler(deps))
		grp.GET("/:id/credentials", PluginCredentialGetHandler(deps))
		grp.POST("/:id/credentials/rotate", PluginCredentialRotateHandler(deps))
		grp.DELETE("/:id/tenant_config", PluginTenantDeleteHandler(deps))
	}
}
