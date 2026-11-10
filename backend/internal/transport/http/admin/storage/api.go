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
	g.GET("/execution-control", h.executionConfiguration)
	g.POST("/execution-control/changes", h.changeExecution)
	g.GET("/execution-control/requests/:request_id", h.executionRequest)
	g.GET("/overview", h.overview)
	g.GET("/database-tables", h.databaseTables)
	g.GET("/database-tables/monitoring", h.tableMonitoring)
	g.PUT("/database-tables/monitoring", h.saveTableMonitoring)
	g.POST("/database-tables/monitoring/adoptions", h.adoptTableMonitoring)
	g.GET("/database-tables/monitoring/requests/:request_id", h.tableMonitoringRequest)
	g.GET("/database-tables/:table_id/history", h.tableHistory)
	g.GET("/database-tables/:table_id/retention", h.systemTablePolicy)
	g.PUT("/database-tables/:table_id/retention", h.saveSystemTablePolicy)
	g.POST("/database-tables/:table_id/retention/adoptions", h.adoptSystemTableRetention)
	g.POST("/database-tables/:table_id/retention/changes", h.changeSystemTableRetention)
	g.GET("/database-tables/:table_id/retention/requests/:request_id", h.systemTableRetentionRequest)
	g.GET("/database-tables/:table_id/retention/runs/:request_id", h.systemTableRetentionRunRequest)
	g.POST("/database-tables/:table_id/retention/previews", h.systemTablePreview)
	g.POST("/database-tables/:table_id/retention/runs", h.systemTableRun)
	g.GET("/database-tables/:table_id", h.databaseTables)
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
	g.POST("/database-retention/adoptions", h.databaseAdoption)
	g.POST("/database-retention/changes", h.databaseChange)
	g.GET("/database-retention/requests/:request_id", h.databaseRequest)
	g.GET("/database-retention/runs/:request_id", h.databaseRunRequest)
	g.POST("/database-retention/previews", h.databasePreview)
	g.POST("/database-retention/runs", h.databaseRun)
	g.GET("/runtime", h.runtimeStatus)
	g.PUT("/runtime/budget", h.saveRuntimeBudget)
	g.GET("/runtime/budget", h.budgetConfiguration)
	g.POST("/runtime/budget/adoptions", h.adoptBudget)
	g.GET("/runtime/budget/requests/:request_id", h.budgetRequest)
	g.PUT("/runtime/policies/:source_id", h.saveRuntimePolicy)
	g.GET("/runtime/policies/:source_id", h.lifecycleConfiguration)
	g.POST("/runtime/policies/:source_id/adoptions", h.adoptLifecycle)
	g.GET("/runtime/policies/:source_id/requests/:request_id", h.lifecycleRequest)
	g.POST("/runtime/previews", h.runtimePreview)
	g.POST("/runtime/runs", h.runtimeRun)
	g.GET("/runtime/events", h.runtimeEvents)
	g.POST("/runtime/runs/:run_id/acknowledge", h.runtimeAcknowledge)
}
