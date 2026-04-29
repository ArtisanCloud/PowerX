package plugin_runtime

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

// RegisterTenantRoutes registers plugin-facing tenant runtime APIs under /tenant/plugin-runtime.
func RegisterTenantRoutes(group *gin.RouterGroup, deps *shared.Deps) {
	if group == nil || deps == nil || deps.DB == nil {
		return
	}
	h := newTenantRuntimeHandler(deps)
	if h == nil {
		return
	}
	tenantGroup := group.Group("/tenant/plugin-runtime")
	{
		tenantGroup.GET("/knowledge-spaces", h.ListKnowledgeSpaces)
		tenantGroup.POST("/agents/instantiate", h.InstantiateAgent)
		tenantGroup.GET("/agents", h.ListAgents)
	}
}
