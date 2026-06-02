package root

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	"github.com/gin-gonic/gin"
)

func RegisterAPIRoutes(_ *gin.RouterGroup, protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	if protectedGroup == nil || deps == nil || deps.DB == nil {
		return
	}
	h := NewSupportSessionHandler(iamsvc.NewRootSupportSessionService(deps.DB))
	g := protectedGroup.Group("/admin/root/support-sessions")
	{
		g.POST("", h.Start)
		g.POST("/:id/end", h.End)
	}
}
