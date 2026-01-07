package knowledge_space

import (
	"errors"
	"net/http"
	"strings"

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
	format := strings.TrimSpace(req.Format)
	if format == "" {
		format = strings.TrimSpace(req.SourceType)
	}
	if format == "" {
		dto.ResponseError(c, http.StatusBadRequest, "缺少文档格式(format/sourceType)", errors.New("missing format"))
		return
	}
	job, err := h.svc.Trigger(c.Request.Context(), ksvc.TriggerIngestionInput{
		SpaceID:          spaceID,
		Format:           format,
		SourceURI:        req.SourceURI,
		IngestionProfile: req.IngestionProfile,
		ProcessorProfile: req.ProcessorProfile,
		OCRRequired:      req.OCRRequired,
		MaskingProfile:   req.MaskingProfile,
		Priority:         req.Priority,
		RequestedBy:      req.RequestedBy,
	})
	if err != nil {
		switch {
		case errors.Is(err, ksvc.ErrInvalidInput):
			dto.ResponseError(c, http.StatusBadRequest, "入库参数不合法", err)
		case errors.Is(err, ksvc.ErrSpaceNotFound):
			dto.ResponseError(c, http.StatusNotFound, "知识空间不存在或已退役", err)
		default:
			dto.ResponseError(c, http.StatusInternalServerError, "触发入库失败", err)
		}
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, toIngestionJobView(job))
}
