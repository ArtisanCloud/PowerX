package http

import (
	"github.com/ArtisanCloud/PowerX/api/http/admin/agent"
	admin "github.com/ArtisanCloud/PowerX/api/http/admin/plugin"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	"github.com/gin-gonic/gin"
)

// RegisterAPIRoutes 负责挂载所有业务路由
func RegisterAPIRoutes(r *gin.Engine, authMiddleware gin.HandlerFunc, cfg *config.Config) {
	prefix := cfg.Server.APIPrefix
	if prefix == "" {
		prefix = "/api"
	}
	publicGroup := r.Group(prefix)
	// 公开健康检查
	publicGroup.GET("/health", HealthHandler)

	// 公开的JWT令牌生成端点（仅用于开发测试）
	publicGroup.POST("/auth/generate_token", GenerateTokenHandler(cfg))

	// 受保护的API组
	protectedGroup := r.Group(prefix)
	protectedGroup.Use(authMiddleware)

	// 如果你已有 Admin 的中间件（鉴权/租户），可以在这里加到 Group 上
	mgr := manager.GetPluginManager()
	h := admin.NewPluginHandler(mgr, cfg.Plugin.BasePrefix)

	grp := protectedGroup.Group("/admin/plugins")
	{
		grp.GET("/", h.List)                // GET  /api/v1/admin/plugins
		grp.GET("/menus", h.Menus)          // GET  /api/v1/admin/plugins/menus
		grp.POST("/:id/enable", h.Enable)   // POST /api/v1/admin/plugins/:id/enable
		grp.POST("/:id/disable", h.Disable) // POST /api/v1/admin/plugins/:id/disable
	}

	agent.RegisterAPIRoutes(publicGroup, protectedGroup)
}
