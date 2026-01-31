package openapi

import (
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	httpmiddleware "github.com/ArtisanCloud/PowerX/internal/http/middleware"
	agentOpenAPI "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/agent"
	aiOpenAPI "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/ai"
	capabilityRegistryOpenAPI "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/capability_registry"
	integrationGatewayOpenAPI "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/integration_gateway"
	knowledgeSpaceOpenAPI "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/knowledge_space"
	mediaOpenAPI "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/media"
	pluginReleaseOpenAPI "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/plugin_release"
	"github.com/gin-gonic/gin"
)

// RegisterAPIRoutes 挂载开放平台（租户/公开）HTTP 路由。
func RegisterAPIRoutes(
	r *gin.Engine, authMiddleware gin.HandlerFunc,
	cfg *config.Config, deps *shared.Deps,
) {
	if r == nil || cfg == nil {
		return
	}
	prefix := cfg.Server.APIPrefix
	if prefix == "" {
		prefix = "/api"
	}
	publicGroup := r.Group(prefix)
	protectedGroup := r.Group(prefix)
	if authMiddleware != nil {
		protectedGroup.Use(authMiddleware)
	}

	httpmiddleware.RegisterLocalUploadEndpoint(protectedGroup, cfg, deps.MediaSvc)

	mediaOpenAPI.RegisterPublicResource(r, deps)

	agentOpenAPI.Register(publicGroup, protectedGroup, deps)
	capabilityRegistryOpenAPI.RegisterTenantRoutes(protectedGroup, deps)
	integrationGatewayOpenAPI.RegisterTenantRoutes(protectedGroup, deps)
	aiOpenAPI.Register(publicGroup, protectedGroup, deps)
	pluginReleaseOpenAPI.RegisterTenantRoutes(protectedGroup, deps)
	knowledgeSpaceOpenAPI.Register(publicGroup, protectedGroup, deps)
	mediaOpenAPI.Register(publicGroup, protectedGroup, deps)
}
