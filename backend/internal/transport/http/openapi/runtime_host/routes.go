package runtime_host

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

func RegisterTenantRoutes(rg *gin.RouterGroup, deps *shared.Deps) {
	h := NewHandler(deps.RuntimeHostSvc)
	g := rg.Group("/tenant/runtime")
	g.GET("/cache/entries", h.GetCache)
	g.PUT("/cache/entries", h.SetCache)
	g.DELETE("/cache/entries", h.DeleteCache)
	g.POST("/tasks", h.CreateTask)
	g.GET("/tasks/:task_uuid", h.GetTask)
	g.PATCH("/tasks/:task_uuid", h.UpdateTask)
}
