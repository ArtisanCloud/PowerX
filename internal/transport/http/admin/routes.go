package http

import (
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/agent"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/auth"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/iam"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/menu"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/plugin"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/system"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/tenants"
	"github.com/gin-gonic/gin"
)

// RegisterAPIRoutes 负责挂载所有业务路由
func RegisterAPIRoutes(
	r *gin.Engine, authMiddleware gin.HandlerFunc,
	cfg *config.Config, deps *shared.Deps,
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

	system.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	tenants.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	iam.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	menu.RegisterAPIRoutes(publicGroup, protectedGroup)
	auth.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	agent.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	plugin.RegisterAPIRoutes(publicGroup, protectedGroup)

}
