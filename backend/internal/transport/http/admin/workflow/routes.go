package workflow

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

// RegisterAPIRoutes 注册工作流相关 HTTP 管理端路由。
func RegisterAPIRoutes(publicGroup, protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	if deps == nil || deps.Workflow == nil || deps.Workflow.Service == nil {
		return
	}

	handler := NewHandler(deps.Workflow.Service)
	group := protectedGroup.Group("/admin/workflows")

	group.POST("/definitions", handler.CreateDefinition)
	group.GET("/definitions", handler.ListDefinitions)
	group.GET("/definitions/:definition_uuid", handler.GetDefinition)
	group.POST("/definitions/:definition_uuid/publish", handler.PublishDefinition)
	group.POST("/definitions/:definition_uuid/validate", handler.ValidateDefinition)

	group.POST("/instances", handler.StartInstance)
	group.GET("/instances", handler.ListInstances)
	group.GET("/instances/export", handler.ExportInstances)
	group.GET("/instances/:instance_uuid", handler.GetInstance)
	group.POST("/instances/:instance_uuid/actions", handler.ControlInstance)

	group.GET("/node-catalog", handler.ListNodeCatalog)
	group.GET("/node-catalog/:node_kind", handler.GetNodeCatalogItem)

	group.GET("/review-tasks", handler.ListHumanReviewTasks)
	group.GET("/review-tasks/:review_task_uuid", handler.GetHumanReviewTask)
	group.POST("/review-tasks/:review_task_uuid/actions", handler.ActHumanReviewTask)

	group.GET("/packs", handler.ListWorkflowPacks)
	group.POST("/packs/seed", requireWorkflowTenantAdmin(deps), handler.SeedWorkflowPacks)
	group.GET("/packs/:workflow_key", handler.GetWorkflowPack)
}
