package storage

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

// RegisterAPIRoutes 挂载 Root 专用存储观测、草稿和显式启用的运行治理。
func RegisterAPIRoutes(_ *gin.RouterGroup, protected *gin.RouterGroup, deps *shared.Deps) {
	if protected == nil || deps == nil {
		return
	}
	h := &handler{service: deps.StorageGovernance}
	g := protected.Group("/admin/ops/storage")
	g.Use(requireRootUser)
	g.GET("/overview", h.overview)
	g.POST("/estimates", h.estimate)
	g.GET("/policy", h.policy)
	g.PUT("/policy", h.savePolicy)
	g.GET("/history", h.history)
	g.POST("/cleanup-previews", h.previewCleanup)
	g.POST("/cleanup-runs", h.executeCleanup)
	g.GET("/cleanup-runs", h.cleanupRuns)
	g.GET("/cleanup-runs/:run_id", h.cleanupRun)
	g.GET("/database-retention", h.databasePolicy)
	g.PUT("/database-retention", h.saveDatabasePolicy)
	g.POST("/database-retention/previews", h.databasePreview)
	g.POST("/database-retention/runs", h.databaseRun)
	g.GET("/runtime", h.runtimeStatus)
	g.PUT("/runtime/budget", h.saveRuntimeBudget)
	g.PUT("/runtime/policies/:source_id", h.saveRuntimePolicy)
	g.POST("/runtime/previews", h.runtimePreview)
	g.POST("/runtime/runs", h.runtimeRun)
	g.GET("/runtime/events", h.runtimeEvents)
	g.POST("/runtime/runs/:run_id/acknowledge", h.runtimeAcknowledge)
}
