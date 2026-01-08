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
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// RegisterAPIRoutes wires admin HTTP endpoints.
func RegisterAPIRoutes(public, protected *gin.RouterGroup, deps *shared.Deps) {
	if protected == nil || deps == nil || deps.KnowledgeSpace == nil || deps.KnowledgeSpace.Service == nil {
		return
	}
	handler := &Handler{svc: deps.KnowledgeSpace.Service}
	profileHandler := newProfileHandler(deps.DB)
	corpusCheckHandler := newCorpusCheckHandler(deps.KnowledgeSpace.CorpusCheck)
	playgroundHandler := newPlaygroundHandler(deps.DB, deps.KnowledgeSpace.VectorStore)
	ingestionHandler := NewIngestionHandler(deps)
	fusionHandler := NewFusionHandler(deps)
	feedbackHandler := NewFeedbackHandler(deps)
	deltaHandler := NewDeltaHandler(deps)
	eventHandler := NewEventHandler(deps)
	decayHandler := NewDecayHandler(deps)
	releaseHandler := NewReleaseHandler(deps)
	group := protected.Group("/admin/knowledge-spaces")
	{
		group.POST("", handler.create)
		group.PATCH("/:spaceId", handler.update)
		group.POST("/:spaceId/retire", handler.retire)
		if corpusCheckHandler != nil {
			group.POST("/:spaceId/corpus-check/jobs", corpusCheckHandler.Start)
			group.GET("/:spaceId/corpus-check/jobs/:jobId", corpusCheckHandler.Get)
		}
		if playgroundHandler != nil {
			group.POST("/:spaceId/playground/retrieval", playgroundHandler.Retrieve)
		}
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
			group.POST("/:spaceId/feedback/:caseId/close", feedbackHandler.Close)
			group.POST("/:spaceId/feedback/:caseId/escalate", feedbackHandler.Escalate)
			group.POST("/:spaceId/feedback/:caseId/reprocess", feedbackHandler.Reprocess)
			group.POST("/:spaceId/feedback/:caseId/rollback", feedbackHandler.Rollback)
			group.GET("/:spaceId/feedback/export", feedbackHandler.Export)
		}
	}
	if profileHandler != nil {
		profileGroup := protected.Group("/admin/knowledge/profiles")
		profileHandler.routes(profileGroup)
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
	if eventHandler != nil {
		eventGroup := protected.Group("/knowledge/events")
		{
			eventGroup.POST("/apply", eventHandler.Apply)
			eventGroup.POST("/retry", eventHandler.Retry)
		}
		protected.POST("/knowledge/index/hot-update", eventHandler.HotUpdate)
		protected.POST("/agent/weights/refresh", eventHandler.RefreshAgent)
	}
	if decayHandler != nil {
		decayGroup := protected.Group("/knowledge/decay")
		{
			decayGroup.POST("/tasks", decayHandler.Scan)
			decayGroup.POST("/restore", decayHandler.Restore)
			decayGroup.GET("/status", decayHandler.Status)
		}
	}
	if releaseHandler != nil {
		releaseGroup := protected.Group("/knowledge/release")
		{
			releaseGroup.GET("/policies", releaseHandler.ListPolicies)
			releaseGroup.GET("/status", releaseHandler.Status)
			releaseGroup.POST("/policies", releaseHandler.UpsertPolicy)
			releaseGroup.POST("/publish", releaseHandler.Publish)
			releaseGroup.POST("/promote", releaseHandler.Promote)
			releaseGroup.POST("/rollback", releaseHandler.Rollback)
		}
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
	tenantUUID, ok := tenantUUIDFromContext(c)
	if !ok {
		return
	}
	policyID, err := strconv.ParseUint(strings.TrimSpace(req.PolicyTemplateVersionID), 10, 64)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的策略模版 ID", err)
		return
	}
	flags := ksvc.EncodeConcurrencyFlag(req.FeatureFlags, req.Quotas.IngestionConcurrency)
	space, err := h.svc.CreateSpace(c.Request.Context(), ksvc.CreateSpaceInput{
		TenantUUID:     tenantUUID.String(),
		SpaceName:      req.SpaceName,
		DepartmentCode: req.DepartmentCode,
		QuotaCPU:       req.Quotas.CPUCores,
		QuotaStorageGB: req.Quotas.StorageGB,
		PolicyVersion:  policyID,
		IngestionProfileKey: strings.TrimSpace(req.IngestionProfileKey),
		IndexProfileKey:     strings.TrimSpace(req.IndexProfileKey),
		RAGProfileKey:       strings.TrimSpace(req.RAGProfileKey),
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

func tenantUUIDFromContext(c *gin.Context) (uuid.UUID, bool) {
	tenantUUID, err := reqctx.RequireTenantUUIDValueFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少租户上下文", err)
		return uuid.Nil, false
	}
	return tenantUUID, true
}
