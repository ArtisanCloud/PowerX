package knowledge_space

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	ksdelta "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/delta"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// DeltaHandler exposes HTTP endpoints for delta sync orchestration.
type DeltaHandler struct {
	svc *ksdelta.Service
}

// NewDeltaHandler builds a handler when dependencies are available.
func NewDeltaHandler(deps *shared.Deps) *DeltaHandler {
	if deps == nil || deps.KnowledgeSpace == nil || deps.KnowledgeSpace.Delta == nil {
		return nil
	}
	return &DeltaHandler{svc: deps.KnowledgeSpace.Delta}
}

func (h *DeltaHandler) Start(c *gin.Context) {
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "delta service unavailable", nil)
		return
	}
	var req startDeltaJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(req.SpaceID))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的空间 ID", err)
		return
	}
	job, err := h.svc.StartJob(c.Request.Context(), ksdelta.StartJobInput{
		SpaceID:      spaceID,
		Source:       req.Source,
		PackageURI:   req.PackageURI,
		RequestedBy:  req.RequestedBy,
		DiffAccuracy: req.DiffAccuracy,
		Notes:        req.Notes,
	})
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, toDeltaJobView(job))
}

func (h *DeltaHandler) Report(c *gin.Context) {
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "delta service unavailable", nil)
		return
	}
	jobID, err := uuid.Parse(strings.TrimSpace(c.Param("jobId")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的任务 ID", err)
		return
	}
	job, err := h.svc.GetReport(c.Request.Context(), jobID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccess(c, toDeltaJobView(job))
}

func (h *DeltaHandler) Publish(c *gin.Context) {
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "delta service unavailable", nil)
		return
	}
	var req publishDeltaJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	jobID, err := uuid.Parse(strings.TrimSpace(req.JobID))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的任务 ID", err)
		return
	}
	job, err := h.svc.Publish(c.Request.Context(), ksdelta.PublishJobInput{
		JobID:          jobID,
		Decision:       req.Decision,
		ApprovedBy:     req.ApprovedBy,
		DiffAccuracy:   req.DiffAccuracy,
		PartialRelease: req.PartialRelease,
	})
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccess(c, toDeltaJobView(job))
}

func (h *DeltaHandler) Rollback(c *gin.Context) {
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "delta service unavailable", nil)
		return
	}
	var req rollbackDeltaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	jobID, err := uuid.Parse(strings.TrimSpace(req.JobID))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的任务 ID", err)
		return
	}
	job, err := h.svc.Rollback(c.Request.Context(), ksdelta.RollbackInput{
		JobID:       jobID,
		RequestedBy: req.RequestedBy,
		Reason:      req.Reason,
	})
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccess(c, toDeltaJobView(job))
}

func (h *DeltaHandler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ksdelta.ErrInvalidInput), errors.Is(err, ksdelta.ErrUnknownSource):
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
	case errors.Is(err, ksdelta.ErrJobConflict):
		dto.ResponseError(c, http.StatusConflict, err.Error(), err)
	case errors.Is(err, ksdelta.ErrSpaceNotFound), errors.Is(err, ksdelta.ErrJobNotFound):
		dto.ResponseError(c, http.StatusNotFound, err.Error(), err)
	case errors.Is(err, ksdelta.ErrPartialReleaseDenied):
		dto.ResponseError(c, http.StatusForbidden, err.Error(), err)
	default:
		dto.ResponseError(c, http.StatusInternalServerError, err.Error(), err)
	}
}
