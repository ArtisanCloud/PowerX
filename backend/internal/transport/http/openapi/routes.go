package openapi

import (
	"context"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	httpmiddleware "github.com/ArtisanCloud/PowerX/internal/http/middleware"
	customerSvc "github.com/ArtisanCloud/PowerX/internal/service/customer"
	metadataAdmin "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/metadata"
	agentOpenAPI "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/agent"
	agentSessionOpenAPI "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/agent_session"
	aiOpenAPI "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/ai"
	capabilityRegistryOpenAPI "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/capability_registry"
	customerOpenAPI "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/customer"
	iamOpenAPI "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/iam"
	integrationGatewayOpenAPI "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/integration_gateway"
	knowledgeSpaceOpenAPI "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/knowledge_space"
	mediaOpenAPI "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/media"
	notificationsOpenAPI "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/notifications"
	requestIdentityOpenAPI "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/plugin_identity"
	pluginReleaseOpenAPI "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/plugin_release"
	pluginRuntimeOpenAPI "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/plugin_runtime"
	runtimeHostOpenAPI "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/runtime_host"
	skillsOpenAPI "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/skills"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
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

	httpmiddleware.RegisterLocalUploadEndpoint(publicGroup, cfg, deps.MediaSvc)

	mediaOpenAPI.RegisterPublicResource(r, deps)

	agentOpenAPI.Register(publicGroup, protectedGroup, deps)
	agentSessionOpenAPI.NewHandler(deps.AgentSessionSvc).Register(protectedGroup)
	capabilityRegistryOpenAPI.RegisterTenantRoutes(protectedGroup, deps)
	customerOpenAPI.RegisterTenantRoutes(protectedGroup, deps, customerOpenAPI.WithClientApps(customerClientApps(cfg)))
	iamOpenAPI.RegisterTenantRoutes(protectedGroup, deps)
	requestIdentityOpenAPI.Register(protectedGroup, deps)
	skillsOpenAPI.RegisterTenantRoutes(protectedGroup, deps)
	integrationGatewayOpenAPI.RegisterTenantRoutes(protectedGroup, deps)
	notificationsOpenAPI.RegisterTenantRoutes(protectedGroup, deps)
	aiOpenAPI.Register(publicGroup, protectedGroup, deps)
	pluginReleaseOpenAPI.RegisterTenantRoutes(protectedGroup, deps)
	pluginRuntimeOpenAPI.RegisterTenantRoutes(protectedGroup, deps)
	knowledgeSpaceOpenAPI.Register(publicGroup, protectedGroup, deps)
	mediaOpenAPI.Register(publicGroup, protectedGroup, deps)
	metadataAdmin.RegisterTenantHostRoutes(protectedGroup, deps)
	runtimeHostOpenAPI.RegisterTenantRoutes(protectedGroup, deps)
}

// customerClientApps 把配置中的客户端应用注册表转换为客户服务使用的注册表。
// 尚未填写密钥的应用照常登记，登录时明确返回未配置，并在启动时提示。
func customerClientApps(cfg *config.Config) customerSvc.ClientAppRegistry {
	apps := make([]customerSvc.ClientApp, 0, len(cfg.CustomerAuth.ClientApps))
	for _, a := range cfg.CustomerAuth.ClientApps {
		if a.Secret() == "" {
			logger.WarnF(context.Background(), "customer_auth.client_apps %s/%s 未配置密钥，该应用的登录会返回 CUSTOMER_CLIENT_APP_NOT_CONFIGURED", a.Platform, a.AppID)
		}
		apps = append(apps, customerSvc.ClientApp{Platform: a.Platform, AppID: a.AppID, Name: a.Name, Secret: a.Secret(), PluginIDs: a.PluginIDs, TenantUUIDs: a.TenantUUIDs})
	}
	return customerSvc.NewStaticClientAppRegistry(apps)
}
