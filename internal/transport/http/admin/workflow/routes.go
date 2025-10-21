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
	group.GET("/definitions/:definitionId", handler.GetDefinition)
	group.POST("/definitions/:definitionId/publish", handler.PublishDefinition)

	group.POST("/instances", handler.StartInstance)
	group.GET("/instances/:instanceId", handler.GetInstance)
	group.POST("/instances/:instanceId/actions", handler.ControlInstance)
}
