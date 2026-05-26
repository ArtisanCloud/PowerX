package scheduler

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

func RegisterAPIRoutes(publicGroup, protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	if protectedGroup == nil {
		return
	}
	h := NewHandler(deps)
	if h == nil {
		return
	}
	g := protectedGroup.Group("/admin/scheduler")
	g.GET("/jobs", h.ListJobs)
	g.POST("/jobs", h.CreateJob)
	g.GET("/jobs/:job_id", h.GetJob)
	g.PATCH("/jobs/:job_id", h.UpdateJob)
	g.POST("/jobs/:job_id/pause", h.PauseJob)
	g.POST("/jobs/:job_id/resume", h.ResumeJob)
	g.POST("/jobs/:job_id/trigger", h.TriggerJob)
	g.GET("/jobs/:job_id/runs", h.ListRuns)
}
