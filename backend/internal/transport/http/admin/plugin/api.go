package plugin

import (
	"context"
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	pluginservice "github.com/ArtisanCloud/PowerX/internal/service/plugin"
	modelIAM "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	coreiam "github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
)

func RegisterAPIRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	cfg := config.GetGlobalConfig()
	SetMarketplaceBasePrefix(cfg.Plugin.BasePrefix)
	tenantSvc := pluginservice.NewTenantPluginInstanceService(deps.DB)
	SetTenantPluginEnabledChecker(tenantSvc.IsEnabled)

	grp := protectedGroup.Group("/admin/plugins")
	tenantAdmin := pluginTenantAdminMiddleware(deps)
	rootOnly := pluginRootOnlyMiddleware()
	{
		publicGroup.GET("/public/plugins/:id/icon", PluginIconHandler)

		// 统一使用 v2 响应格式，仅保留一个入口
		grp.GET("/marketplace/plugins", tenantAdmin, MarketplaceListV2Handler(cfg.Plugin.BasePrefix, deps))
		grp.GET("/tenant-instances", tenantAdmin, TenantPluginInstanceListHandler(deps))
		grp.POST("/tenant-instances/:plugin_id/enable", tenantAdmin, TenantPluginInstanceEnableHandler(deps))
		grp.POST("/tenant-instances/:plugin_id/disable", tenantAdmin, TenantPluginInstanceDisableHandler(deps))

		grp.GET("/", tenantAdmin, PluginListHandler)       // GET  /api/v1/admin/plugins
		grp.GET("/menus", tenantAdmin, PluginMenusHandler) // GET  /api/v1/admin/plugins/menus

		// 系统级：启停/重启/状态/安装/卸载/切换版本
		grp.POST("/:id/enable", rootOnly, PluginEnableHandler)                // POST /api/v1/admin/plugins/:id/enable
		grp.POST("/:id/disable", rootOnly, PluginDisableHandler)              // POST /api/v1/admin/plugins/:id/disable
		grp.POST("/:id/restart", rootOnly, PluginRestartHandler)              // POST /api/v1/admin/plugins/:id/restart
		grp.GET("/:id/status", tenantAdmin, PluginStatusHandler)              // GET /api/v1/admin/plugins/:id/status
		grp.GET("/:id/logs", rootOnly, PluginLogsHandler)                     // GET /api/v1/admin/plugins/:id/logs
		grp.POST("/:id/switch_version", rootOnly, PluginSwitchVersionHandler) // POST /api/v1/admin/plugins/:id/switch_version
		grp.POST("/:id/uninstall", rootOnly, PluginUninstallHandler(deps))    // POST /api/v1/admin/plugins/:id/uninstall
		grp.POST("/:id/drain", rootOnly, PluginDrainCreateHandler(deps))      // POST /api/v1/admin/plugins/:id/drain
		grp.GET("/:id/drain", rootOnly, PluginDrainListHandler(deps))         // GET  /api/v1/admin/plugins/:id/drain
		grp.POST("/drain/:job_id/refresh", rootOnly, PluginDrainRefreshHandler(deps))
		grp.GET("/:id/drain/blockers", rootOnly, PluginDrainBlockersListHandler(deps))
		grp.POST("/:id/drain/cancel_blockers", rootOnly, PluginDrainCancelBlockersHandler(deps))
		grp.GET("/drain/:job_id", rootOnly, PluginDrainGetHandler(deps)) // GET  /api/v1/admin/plugins/drain/:job_id

		grp.POST("/install/local", rootOnly, PluginInstallLocalHandler) // POST /api/v1/admin/plugins/install/local
		grp.POST("/install/url", rootOnly, PluginInstallURLHandler)     // POST /api/v1/admin/plugins/install/url

		// 租户级：查询配置、启用/停用（仅影响本租户）、凭证查看与轮换
		grp.GET("/:id/tenant_config", tenantAdmin, PluginTenantConfigHandler(deps))
		grp.POST("/:id/tenant_enable", tenantAdmin, PluginTenantEnableHandler(deps))
		grp.GET("/:id/credentials", tenantAdmin, PluginCredentialGetHandler(deps))
		grp.POST("/:id/credentials/rotate", tenantAdmin, PluginCredentialRotateHandler(deps))
		grp.DELETE("/:id/tenant_config", tenantAdmin, PluginTenantDeleteHandler(deps))
		grp.GET("/:id", tenantAdmin, PluginGetHandler) // GET /api/v1/admin/plugins/:id
	}
}

func pluginTenantAdminMiddleware(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if reqctx.IsRoot(c.Request.Context()) || hasPluginTenantAdminRole(c.Request.Context(), deps) {
			c.Next()
			return
		}
		dto.ResponseError(c, http.StatusForbidden, "tenant owner/admin only", nil)
		c.Abort()
	}
}

func pluginRootOnlyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !reqctx.IsRoot(c.Request.Context()) {
			dto.ResponseError(c, http.StatusForbidden, "root only", nil)
			c.Abort()
			return
		}
		c.Next()
	}
}

func hasPluginTenantAdminRole(ctx context.Context, deps *shared.Deps) bool {
	if deps == nil || deps.DB == nil {
		return false
	}
	tenantUUID := strings.TrimSpace(reqctx.GetTenantUUID(ctx))
	memberID := reqctx.GetMemberID(ctx)
	if tenantUUID == "" || memberID == 0 {
		return false
	}

	tRB := (&modelIAM.RoleBinding{}).GetTableName(true)
	tRole := (&modelIAM.Role{}).GetTableName(true)

	var count int64
	err := deps.DB.WithContext(ctx).
		Table(tRB+" AS rb").
		Joins("JOIN "+tRole+" AS r ON r.id = rb.role_id").
		Where("rb.tenant_uuid = ? AND rb.subject_type = ? AND rb.subject_id = ?", tenantUUID, modelIAM.SubMember, memberID).
		Where("r.scope = ? AND r.code IN ?", string(coreiam.RoleScopeTenant), []string{string(coreiam.CodeRoleOwner), string(coreiam.CodeRoleAdmin)}).
		Count(&count).Error
	if err != nil {
		logger.WarnF(ctx, "[plugins] tenant role check failed tenant=%s member=%d err=%v", tenantUUID, memberID, err)
		return false
	}
	return count > 0
}
