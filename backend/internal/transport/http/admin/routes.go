package http

import (
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	httpmiddleware "github.com/ArtisanCloud/PowerX/internal/http/middleware"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/agent"
	agentmodelhubHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/agent_model_hub"
	agentlifecycleHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/agentlifecycle"
	agenttraceHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/agenttrace"
	backupHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/backup"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/capability"
	capabilityRegistryHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/capability_registry"
	customerHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/customer"
	deployHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/deploy"
	devHotloadHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/dev_hotload"
	eventFabricHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/event_fabric"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/iam"
	integrationGatewayHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/integration_gateway"
	knowledgeSpaceHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/knowledge_space"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/media"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/menu"
	metadataHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/metadata"
	migrationHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/migration"
	monitorHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/monitor"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/notifications"
	opsHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/ops"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/plugin"
	pluginDevHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/plugin_dev"
	pluginReleaseHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/plugin_release"
	pluginSandboxHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/plugin_sandbox"
	rootHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/root"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/runtime"
	schedulerHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/scheduler"
	skillsHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/skills"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/system"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/tenants"
	userauth "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/user/auth"
	versionHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/version"
	workflowHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/workflow"
	publicSaaS "github.com/ArtisanCloud/PowerX/internal/transport/http/public/saas"
	"github.com/gin-gonic/gin"
)

// RegisterAPIRoutes 负责挂载所有业务路由
func RegisterAPIRoutes(
	r *gin.Engine, authMiddleware gin.HandlerFunc,
	cfg *config.Config, deps *shared.Deps,
) {
	prefix := config.ResolveAPIPrefix(cfg)
	// 公开健康检查（兼容不带版本前缀探活）
	r.GET("/healthz", HealthHandler)

	publicGroup := r.Group(prefix)
	// 公开健康检查
	publicGroup.GET("/health", HealthHandler)
	publicSaaS.RegisterAPIRoutes(publicGroup, deps)

	// 受保护的API组
	protectedGroup := r.Group(prefix)
	protectedGroup.Use(authMiddleware)

	// Admin 体系在 /api/{prefix}/admin/... 下提供上传端点
	adminUploadGroup := protectedGroup.Group("/admin")
	httpmiddleware.RegisterLocalUploadEndpoint(adminUploadGroup, cfg, deps.MediaSvc)

	system.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	tenants.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	customerHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	iam.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	menu.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	userauth.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	// Agent admin routes (includes share/revoke APIs under /admin/agents)
	agent.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	agentmodelhubHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	agentlifecycleHTTP.Register(publicGroup, protectedGroup, deps)
	agenttraceHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	media.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	capability.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	capabilityRegistryHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	integrationGatewayHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	plugin.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	pluginReleaseHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	pluginDevHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	devHotloadHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	deployHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	backupHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	migrationHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	pluginSandboxHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	versionHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	eventFabricHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	workflowHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	knowledgeSpaceHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	notifications.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	runtime.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	rootHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	schedulerHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	skillsHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	opsHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	monitorHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
	metadataHTTP.RegisterAPIRoutes(publicGroup, protectedGroup, deps)
}
