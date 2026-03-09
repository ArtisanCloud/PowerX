package skills

import (
	"net/http"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
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

	catalogH := newCatalogHandler(module.db)
	registryH := newRegistryHandler(module.registry, module.importSvc)
	importH := newImportHandler(module.importSvc)
	publishH := newPublishHandler(module.registry, module.lifecycle)
	rollbackH := newRollbackHandler(module.registry, module.lifecycle)
	if catalogH == nil || registryH == nil || importH == nil || publishH == nil || rollbackH == nil {
		return
	}

	group := protectedGroup.Group("/admin/skills")
	group.Use(middleware.AdminOnlyMiddleware())
	group.Use(rootOnlyMiddleware())
	group.GET("/catalog", catalogH.ListCatalog)
	group.GET("", registryH.List)
	group.POST("", registryH.Register)
	group.POST("/import", importH.Import)
	group.POST("/:skillId/publish", publishH.Publish)
	group.POST("/:skillId/rollback", rollbackH.Rollback)
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
