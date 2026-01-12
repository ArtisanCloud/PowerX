package knowledge_space

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	coreRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	knowledgeRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// RegisterAPIRoutes wires admin HTTP endpoints.
func RegisterAPIRoutes(public, protected *gin.RouterGroup, deps *shared.Deps) {
	if protected == nil || deps == nil || deps.KnowledgeSpace == nil || deps.KnowledgeSpace.Service == nil {
		return
	}
	handler := &Handler{
		svc:      deps.KnowledgeSpace.Service,
		spaces:   knowledgeRepo.NewKnowledgeSpaceRepository(deps.DB),
		policies: knowledgeRepo.NewPolicyTemplateRepository(deps.DB),
	}
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
	strategyHandler := NewStrategyHandler(deps)
	sourceHandler := NewSourceHandler(deps)
	group := protected.Group("/admin/knowledge-spaces")
	{
		group.GET("", handler.list)
		group.POST("", handler.create)
		group.PATCH("/:spaceId", handler.update)
		group.POST("/:spaceId/retire", handler.retire)
		if strategyHandler != nil {
			group.POST("/strategy/validate", strategyHandler.Validate)
			group.GET("/:spaceId/strategy/validate", strategyHandler.ValidateForSpace)
		}
		if corpusCheckHandler != nil {
			group.POST("/:spaceId/corpus-check/jobs", corpusCheckHandler.Start)
			group.GET("/:spaceId/corpus-check/jobs/:jobId", corpusCheckHandler.Get)
		}
		if playgroundHandler != nil {
			group.POST("/:spaceId/playground/retrieval", playgroundHandler.Retrieve)
		}
		if ingestionHandler != nil {
			group.GET("/:spaceId/ingestion-jobs", ingestionHandler.List)
			group.GET("/:spaceId/ingestion-jobs/:jobId", ingestionHandler.Get)
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
		if sourceHandler != nil {
			group.GET("/:spaceId/sources", sourceHandler.ListSpaceSources)
			group.POST("/:spaceId/sources/sync-jobs", sourceHandler.CreateSyncJob)
			group.POST("/:spaceId/sources/sync-jobs/:jobId/pause", sourceHandler.PauseSyncJob)
			group.POST("/:spaceId/sources/sync-jobs/:jobId/run", sourceHandler.RunSyncJob)
			group.GET("/:spaceId/sources/sync-jobs/:jobId", sourceHandler.GetSyncJob)
		}
	}
	if profileHandler != nil {
		profileGroup := protected.Group("/admin/knowledge/profiles")
		profileHandler.routes(profileGroup)
	}
	if sourceHandler != nil {
		sourceGroup := protected.Group("/admin/knowledge-sources")
		{
			sourceGroup.GET("/credentials", sourceHandler.ListCredentials)
			sourceGroup.POST("/credentials", sourceHandler.CreateCredential)
			sourceGroup.GET("/connectors", sourceHandler.ListConnectors)
			sourceGroup.POST("/connectors", sourceHandler.CreateConnector)
			sourceGroup.GET("/connectors/:connectorId", sourceHandler.GetConnector)
			sourceGroup.PATCH("/connectors/:connectorId", sourceHandler.UpdateConnector)
			sourceGroup.POST("/connectors/:connectorId/pause", sourceHandler.PauseConnector)
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
	svc      *ksvc.Service
	spaces   *knowledgeRepo.KnowledgeSpaceRepository
	policies *knowledgeRepo.PolicyTemplateRepository
}

func (h *Handler) list(c *gin.Context) {
	tenantUUID, ok := tenantUUIDFromContext(c)
	if !ok {
		return
	}
	limit := 20
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	status := strings.TrimSpace(c.Query("status"))

	if h.spaces == nil {
		dto.ResponseError(c, http.StatusInternalServerError, "服务异常", errors.New("spaces repository unavailable"))
		return
	}
	items, err := h.spaces.ListByTenant(c.Request.Context(), tenantUUID.String(), status, limit)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "服务异常", err)
		return
	}

	out := make([]knowledgeSpaceResponse, 0, len(items))
	for i := range items {
		out = append(out, toResponse(&items[i]))
	}
	dto.ResponseSuccess(c, out)
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
	policyID, err := h.resolvePolicyTemplateVersionID(c, req.PolicyTemplateVersionID)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的策略模版 ID", err)
		return
	}
	flags := ksvc.EncodeConcurrencyFlag(req.FeatureFlags, req.Quotas.IngestionConcurrency)
	space, err := h.svc.CreateSpace(c.Request.Context(), ksvc.CreateSpaceInput{
		TenantUUID:          tenantUUID.String(),
		SpaceName:           req.SpaceName,
		DepartmentCode:      req.DepartmentCode,
		QuotaCPU:            req.Quotas.CPUCores,
		QuotaStorageGB:      req.Quotas.StorageGB,
		PolicyVersion:       policyID,
		IngestionProfileKey: strings.TrimSpace(req.IngestionProfileKey),
		IndexProfileKey:     strings.TrimSpace(req.IndexProfileKey),
		RAGProfileKey:       strings.TrimSpace(req.RAGProfileKey),
		FeatureFlags:        flags,
		RequestedBy:         req.RequestedBy,
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
		if policy, err = h.resolvePolicyTemplateVersionID(c, req.PolicyTemplateVersionID); err != nil {
			dto.ResponseError(c, http.StatusBadRequest, "无效的策略模版 ID", err)
			return
		}
	}
	var flags []string
	if req.Quotas != nil {
		flags = ksvc.EncodeConcurrencyFlag(req.FeatureFlags, targetConcurrency(req.Quotas))
	} else if len(req.FeatureFlags) > 0 {
		flags = req.FeatureFlags
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

func (h *Handler) resolvePolicyTemplateVersionID(c *gin.Context, raw string) (uint64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, errors.New("missing policy template version id")
	}

	// 1) 优先支持数值 ID（兼容现有 handler 行为）
	if id, err := strconv.ParseUint(raw, 10, 64); err == nil && id > 0 {
		return id, nil
	}

	// 2) 兼容 "default-v1" / "default/v1"
	if h.policies == nil {
		return 0, errors.New("policy repository unavailable")
	}

	sepIdx := strings.LastIndex(raw, "-")
	sep := "-"
	if strings.Contains(raw, "/") {
		sepIdx = strings.LastIndex(raw, "/")
		sep = "/"
	}
	if sepIdx <= 0 || sepIdx >= len(raw)-1 {
		return 0, errors.New("policy template version id must be numeric or in '<template>" + sep + "<version>' format")
	}

	name := strings.TrimSpace(raw[:sepIdx])
	version := strings.TrimSpace(raw[sepIdx+1:])
	if name == "" || version == "" {
		return 0, errors.New("invalid policy template ref")
	}

	tpl, err := h.policies.GetByNameVersion(c.Request.Context(), name, version)
	if err != nil {
		return 0, err
	}
	if tpl == nil || tpl.ID == 0 {
		created, err := h.ensurePolicyTemplateVersion(c, name, version)
		if err != nil {
			return 0, err
		}
		if created == nil || created.ID == 0 {
			return 0, errors.New("policy template version not found")
		}
		return created.ID, nil
	}
	return tpl.ID, nil
}

func (h *Handler) ensurePolicyTemplateVersion(c *gin.Context, name, version string) (*models.PolicyTemplateVersion, error) {
	ctx := c.Request.Context()
	if h.policies == nil {
		return nil, errors.New("policy repository unavailable")
	}

	// 二次确认：避免并发下重复创建
	if existing, err := h.policies.GetByNameVersion(ctx, name, version); err != nil {
		return nil, err
	} else if existing != nil && existing.ID > 0 {
		return existing, nil
	}

	now := time.Now().UTC()
	empty := datatypes.JSON([]byte(`{}`))
	fp := sha256.Sum256([]byte(strings.ToLower(name) + ":" + strings.ToLower(version) + ":{}"))
	hash := hex.EncodeToString(fp[:])

	row := &models.PolicyTemplateVersion{
		TemplateName:    name,
		Version:         version,
		RAGProfile:      empty,
		GraphProfile:    empty,
		MaskingProfile:  empty,
		AlertingProfile: empty,
		ApprovedBy:      "auto",
		ApprovedAt:      &now,
		ImmutableHash:   hash,
	}

	if _, err := h.policies.Create(ctx, row); err != nil {
		if coreRepo.IsUniqueViolation(err) {
			return h.policies.GetByNameVersion(ctx, name, version)
		}
		return nil, err
	}
	return row, nil
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
	case errors.Is(err, ksvc.ErrStrategyPrereqFailed):
		dto.ResponseErrorWithDetails(c, http.StatusBadRequest, "策略依赖未满足", err, dto.DetailsOf(err))
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
