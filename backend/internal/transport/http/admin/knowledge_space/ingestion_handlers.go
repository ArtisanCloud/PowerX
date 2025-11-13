package knowledge_space

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// IngestionHandler exposes ingestion job endpoints.
type IngestionHandler struct {
	svc *ksvc.IngestionService
}

func NewIngestionHandler(deps *shared.Deps) *IngestionHandler {
	if deps == nil || deps.KnowledgeSpace == nil {
		return nil
	}
	return &IngestionHandler{svc: deps.KnowledgeSpace.Ingestion}
}

func (h *IngestionHandler) Trigger(c *gin.Context) {
	if h == nil || h.svc == nil {
		c.Status(http.StatusNotImplemented)
		return
	}
	spaceID, err := uuid.Parse(c.Param("spaceId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "空间 ID 无效", err)
		return
	}
	var req ingestionJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	job, err := h.svc.Trigger(c.Request.Context(), ksvc.TriggerIngestionInput{
		SpaceID:        spaceID,
		SourceType:     req.SourceType,
		SourceURI:      req.SourceURI,
		MaskingProfile: req.MaskingProfile,
		Priority:       req.Priority,
		RequestedBy:    req.RequestedBy,
	})
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "触发入库失败", err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, toIngestionJobView(job))
}
