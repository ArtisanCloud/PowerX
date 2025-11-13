package knowledge_space

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
)

// Register mounts read-only knowledge space health endpoints.
func Register(public, protected *gin.RouterGroup, deps *shared.Deps) {
	if protected == nil || deps == nil || deps.DB == nil {
		return
	}
	handler := &openapiHandler{deps: deps}
	group := protected.Group("/openapi/knowledge-spaces")
	{
		group.GET("/status", handler.status)
	}
}

type openapiHandler struct {
	deps *shared.Deps
}

type statusView struct {
	PendingIAM int64 `json:"pendingIam"`
	Active     int64 `json:"active"`
	Retired    int64 `json:"retired"`
}

func (h *openapiHandler) status(c *gin.Context) {
	ctx := c.Request.Context()
	var pending, active, retired int64
	db := h.deps.DB.WithContext(ctx)
	db.Model(&models.KnowledgeSpace{}).Where("status = ?", models.KnowledgeSpaceStatusPending).Count(&pending)
	db.Model(&models.KnowledgeSpace{}).Where("status = ?", models.KnowledgeSpaceStatusActive).Count(&active)
	db.Model(&models.KnowledgeSpace{}).Where("status = ?", models.KnowledgeSpaceStatusRetired).Count(&retired)

	c.JSON(http.StatusOK, statusView{
		PendingIAM: pending,
		Active:     active,
		Retired:    retired,
	})
}
