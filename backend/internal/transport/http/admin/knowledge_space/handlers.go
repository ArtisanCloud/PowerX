package knowledge_space

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// RegisterAPIRoutes wires admin HTTP endpoints.
func RegisterAPIRoutes(public, protected *gin.RouterGroup, deps *shared.Deps) {
	if protected == nil || deps == nil || deps.KnowledgeSpace == nil || deps.KnowledgeSpace.Service == nil {
		return
	}
	handler := &Handler{svc: deps.KnowledgeSpace.Service}
	ingestionHandler := NewIngestionHandler(deps)
	fusionHandler := NewFusionHandler(deps)
	feedbackHandler := NewFeedbackHandler(deps)
	deltaHandler := NewDeltaHandler(deps)
	group := protected.Group("/admin/knowledge-spaces")
	{
		group.POST("", handler.create)
		group.PATCH("/:spaceId", handler.update)
		group.POST("/:spaceId/retire", handler.retire)
		if ingestionHandler != nil {
			group.POST("/:spaceId/ingestion-jobs", ingestionHandler.Trigger)
		}
		if fusionHandler != nil {
			group.GET("/:spaceId/fusion-strategies", fusionHandler.List)
			group.POST("/:spaceId/fusion-strategies", fusionHandler.Publish)
			group.POST("/:spaceId/fusion-strategies/:strategyId/rollback", fusionHandler.Rollback)
		}
		if feedbackHandler != nil {
			group.GET("/:spaceId/feedback", feedbackHandler.List)
			group.POST("/:spaceId/feedback", feedbackHandler.Submit)
		}
	}
	if deltaHandler != nil {
		deltaGroup := protected.Group("/knowledge/delta")
		{
			deltaGroup.POST("/jobs", deltaHandler.Start)
			deltaGroup.GET("/reports/:jobId", deltaHandler.Report)
			deltaGroup.POST("/publish", deltaHandler.Publish)
		}
		protected.POST("/knowledge/version/rollback", deltaHandler.Rollback)
	}
}

// Handler exposes provisioning handlers.
type Handler struct {
	svc *ksvc.Service
}

func (h *Handler) create(c *gin.Context) {
	var req createSpaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	tenantID, err := uuid.Parse(req.TenantID)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的 tenantId", err)
		return
	}
	policyID, err := strconv.ParseUint(strings.TrimSpace(req.PolicyTemplateVersionID), 10, 64)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的策略模版 ID", err)
		return
	}
	flags := ksvc.EncodeConcurrencyFlag(req.FeatureFlags, req.Quotas.IngestionConcurrency)
	space, err := h.svc.CreateSpace(c.Request.Context(), ksvc.CreateSpaceInput{
		TenantID:       tenantID,
		SpaceName:      req.SpaceName,
		DepartmentCode: req.DepartmentCode,
		QuotaCPU:       req.Quotas.CPUCores,
		QuotaStorageGB: req.Quotas.StorageGB,
		PolicyVersion:  policyID,
		FeatureFlags:   flags,
		RequestedBy:    req.RequestedBy,
	})
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, toResponse(space))
}

func (h *Handler) update(c *gin.Context) {
	var req updateSpaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	spaceID, err := uuid.Parse(c.Param("spaceId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的空间 ID", err)
		return
	}
	var policy uint64
	if strings.TrimSpace(req.PolicyTemplateVersionID) != "" {
		if policy, err = strconv.ParseUint(strings.TrimSpace(req.PolicyTemplateVersionID), 10, 64); err != nil {
			dto.ResponseError(c, http.StatusBadRequest, "无效的策略模版 ID", err)
			return
		}
	}
	var flags []string
	if req.Quotas != nil || len(req.FeatureFlags) > 0 {
		flags = ksvc.EncodeConcurrencyFlag(req.FeatureFlags, targetConcurrency(req.Quotas))
	}
	in := ksvc.UpdateSpaceInput{
		SpaceID:       spaceID,
		PolicyVersion: policy,
		FeatureFlags:  flags,
		Status:        req.Status,
		UpdatedBy:     req.UpdatedBy,
	}
	if req.Quotas != nil {
		in.QuotaCPU = req.Quotas.CPUCores
		in.QuotaStorageGB = req.Quotas.StorageGB
	}
	space, err := h.svc.UpdateSpace(c.Request.Context(), in)
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccess(c, toResponse(space))
}

func (h *Handler) retire(c *gin.Context) {
	var req retireSpaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(c.Param("spaceId")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的空间 ID", err)
		return
	}
	space, err := h.svc.RetireSpace(c.Request.Context(), ksvc.RetireSpaceInput{
		SpaceID:     spaceID,
		Reason:      req.Reason,
		RequestedBy: req.RequestedBy,
	})
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, toResponse(space))
}

func (h *Handler) handleError(c *gin.Context, err error) {
	switch {
	case ksvc.IsConflictError(err):
		dto.ResponseError(c, http.StatusConflict, "同一租户下已存在该空间", err)
	case errors.Is(err, ksvc.ErrProvisioningBusy):
		dto.ResponseError(c, http.StatusConflict, "同一租户正在创建空间，请稍后再试", err)
	case errors.Is(err, ksvc.ErrInvalidInput):
		dto.ResponseError(c, http.StatusBadRequest, "参数不合法", err)
	case errors.Is(err, ksvc.ErrSpaceNotFound):
		dto.ResponseError(c, http.StatusNotFound, "知识空间不存在", err)
	case errors.Is(err, ksvc.ErrInvalidStatusTransition):
		dto.ResponseError(c, http.StatusBadRequest, "状态转换不被允许", err)
	default:
		dto.ResponseError(c, http.StatusInternalServerError, "服务异常", err)
	}
}

func targetConcurrency(q *quotaPayload) int {
	if q == nil || q.IngestionConcurrency <= 0 {
		return 1
	}
	return q.IngestionConcurrency
}
