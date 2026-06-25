package skills

import (
	"net/http"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	adminauthz "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/authz"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

// RegisterAPIRoutes wires skills admin routes.
func RegisterAPIRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	_ = publicGroup
	if protectedGroup == nil || deps == nil || deps.DB == nil {
		return
	}
	module := newModuleDeps(deps.DB)
	if module == nil {
		return
	}

	catalogH := newCatalogHandler(module.db, module.auditSvc)
	registryH := newRegistryHandler(module.registry, module.importSvc)
	importH := newImportHandler(module.importSvc)
	pluginRegistryH := newPluginRegistryHandler(module.importSvc)
	marketplaceH := newMarketplaceHandler(module.importSvc)
	publishH := newPublishHandler(module.registry, module.lifecycle)
	rollbackH := newRollbackHandler(module.registry, module.lifecycle)
	bindingH := newBindingHandler(module.binding, module.auditSvc)
	auditH := newAuditHandler(module.traceRepo, module.auditRepo)
	installTaskH := newInstallTaskHandler(module.installer)
	if catalogH == nil || registryH == nil || importH == nil || pluginRegistryH == nil || marketplaceH == nil || publishH == nil || rollbackH == nil || bindingH == nil || auditH == nil || installTaskH == nil {
		return
	}

	adminGroup := protectedGroup.Group("/admin/skills")

	pluginRegistryGroup := adminGroup.Group("/plugin-registry")
	pluginRegistryGroup.Use(adminauthz.PluginRegistrySyncMiddleware(deps, adminauthz.ScopePluginSkillRegistrySync))
	pluginRegistryGroup.POST("/sync", pluginRegistryH.Sync)
	pluginRegistryGroup.POST("/:skillId/sync", pluginRegistryH.Sync)

	group := adminGroup.Group("")
	group.Use(middleware.AdminOnlyMiddleware())
	group.Use(rootOnlyMiddleware())
	group.GET("/catalog", catalogH.ListCatalog)
	group.POST("/catalog", catalogH.UpsertCatalog)
	group.PATCH("/catalog/:catalogSkillId/active", catalogH.SetCatalogActive)
	group.GET("", registryH.List)
	group.POST("", registryH.Register)
	group.POST("/import", importH.Import)
	group.GET("/marketplace/preview", marketplaceH.Preview)
	group.POST("/:skillId/publish", publishH.Publish)
	group.POST("/:skillId/rollback", rollbackH.Rollback)
	group.POST("/:skillId/bind-capability", bindingH.BindCapability)
	group.GET("/audits", auditH.ListAudits)
	group.GET("/traces", auditH.ListTraces)
	group.GET("/traces/:traceId", auditH.GetTrace)
	group.POST("/install-tasks", installTaskH.Create)
	group.GET("/install-tasks", installTaskH.List)
	group.GET("/install-tasks/:taskId", installTaskH.Get)
}

func rootOnlyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !reqctx.IsRoot(c.Request.Context()) {
			dto.ResponseError(c, http.StatusForbidden, "forbidden: admin root only", nil)
			c.Abort()
			return
		}
		c.Next()
	}
}
