package http

import (
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	httpmiddleware "github.com/ArtisanCloud/PowerX/internal/http/middleware"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/agent"
	agentmodelhubHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/agent_model_hub"
	agentlifecycleHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/agentlifecycle"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/auth"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/capability"
	capabilityRegistryHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/capability_registry"
	devHotloadHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/dev_hotload"
	eventFabricHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/event_fabric"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/iam"
	integrationGatewayHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/integration_gateway"
	knowledgeSpaceHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/knowledge_space"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/media"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/menu"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/plugin"
	pluginDevHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/plugin_dev"
	pluginReleaseHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/plugin_release"
	pluginSandboxHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/plugin_sandbox"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/system"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/tenants"
	versionHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/version"
	workflowHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/workflow"
	knowledgeSpaceOpenAPI "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/knowledge_space"
	"github.com/gin-gonic/gin"
)

// RegisterAPIRoutes 负责挂载所有业务路由
func RegisterAPIRoutes(
	r *gin.Engine, authMiddleware gin.HandlerFunc,
	cfg *config.Config, deps *shared.Deps,
) {
	httpmiddleware.RegisterLocalUploadEndpoint(r, cfg)
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
	// Agent admin routes (includes share/revoke APIs under /admin/agents)
	agent.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	agentmodelhubHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	agentlifecycleHTTP.Register(publicGroup, protectedGroup, deps)
	media.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	capability.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	capabilityRegistryHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	integrationGatewayHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	plugin.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	pluginReleaseHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	pluginDevHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	devHotloadHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	pluginSandboxHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	versionHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	eventFabricHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	workflowHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	knowledgeSpaceHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	knowledgeSpaceOpenAPI.Register(publicGroup, protectedGroup, deps)

}
