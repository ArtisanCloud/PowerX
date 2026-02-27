package websocket

import (
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/transport/websocket/admin/agent"
	"github.com/ArtisanCloud/PowerX/internal/transport/websocket/bus"
	"github.com/gin-gonic/gin"
)

func RegisterWSRoutes(
	r *gin.Engine, authMiddleware gin.HandlerFunc,
	cfg *config.Config, deps *shared.Deps,
) {
	prefix := cfg.Server.WSPrefix
	if prefix == "" {
		prefix = "/wx"
	}

	publicGroup := r.Group(prefix)

	protectedGroup := r.Group(prefix)
	protectedGroup.Use(BearerShim())
	protectedGroup.Use(authMiddleware)

	agent.RegisterWSRoutes(publicGroup, protectedGroup, deps)

	busGroup := r.Group("/api")
	busGroup.Use(BearerShim())
	busGroup.Use(authMiddleware)
	busHandler := bus.NewHandler(deps.DB)
	busGroup.GET("/ws", busHandler.ServeWS)
}
