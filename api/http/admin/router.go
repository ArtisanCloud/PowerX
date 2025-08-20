package http

import (
	"github.com/ArtisanCloud/PowerX/api/http/admin/agent"
	"github.com/ArtisanCloud/PowerX/api/http/admin/auth"
	"github.com/ArtisanCloud/PowerX/api/http/admin/menu"
	"github.com/ArtisanCloud/PowerX/api/http/admin/plugin"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/bootstrap"
	"github.com/gin-gonic/gin"
)

// RegisterAPIRoutes 负责挂载所有业务路由
func RegisterAPIRoutes(
	r *gin.Engine, authMiddleware gin.HandlerFunc,
	cfg *config.Config, deps *bootstrap.Deps,
) {
	prefix := cfg.Server.APIPrefix
	if prefix == "" {
		prefix = "/api"
	}
	publicGroup := r.Group(prefix)
	// 公开健康检查
	publicGroup.GET("/health", HealthHandler)

	// 受保护的API组
	protectedGroup := r.Group(prefix)
	protectedGroup.Use(authMiddleware)

	agent.RegisterAPIRoutes(publicGroup, protectedGroup)
	plugin.RegisterAPIRoutes(publicGroup, protectedGroup)
	menu.RegisterAPIRoutes(publicGroup, protectedGroup)
	auth.RegisterAPIRoutes(publicGroup, protectedGroup, deps.Auth)
}
