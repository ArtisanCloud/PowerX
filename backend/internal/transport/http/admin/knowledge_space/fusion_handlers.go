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
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// FusionHandler exposes HTTP endpoints for fusion strategies.
type FusionHandler struct {
	svc *ksvc.FusionService
}

// NewFusionHandler builds a handler when dependencies are available.
func NewFusionHandler(deps *shared.Deps) *FusionHandler {
	if deps == nil || deps.KnowledgeSpace == nil || deps.KnowledgeSpace.Fusion == nil {
		return nil
	}
	return &FusionHandler{svc: deps.KnowledgeSpace.Fusion}
}

// Publish stores a new fusion strategy version.
func (h *FusionHandler) Publish(c *gin.Context) {
	if h == nil || h.svc == nil {
		c.Status(http.StatusNotImplemented)
		return
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(c.Param("spaceId")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "空间 ID 无效", err)
		return
	}
	var req fusionStrategyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	strategy, err := h.svc.PublishStrategy(c.Request.Context(), ksvc.PublishStrategyInput{
		SpaceID:         spaceID,
		Label:           req.Label,
		BM25Weight:      req.BM25Weight,
		VectorWeight:    req.VectorWeight,
		GraphConstraint: req.GraphConstraint,
		RerankerModel:   req.RerankerModel,
		ConflictPolicy:  req.ConflictPolicy,
		RequestedBy:     req.RequestedBy,
	})
	if err != nil {
		respondFusionError(c, err)
		return
	}
	status := http.StatusCreated
	if strategy.DeploymentState == models.FusionDeploymentDraft {
		status = http.StatusAccepted
	}
	dto.ResponseSuccessWithStatus(c, status, toFusionStrategyResponse(strategy))
}

// List returns existing versions ordered by publish time.
func (h *FusionHandler) List(c *gin.Context) {
	if h == nil || h.svc == nil {
		c.Status(http.StatusNotImplemented)
		return
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(c.Param("spaceId")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "空间 ID 无效", err)
		return
	}
	strategies, err := h.svc.ListStrategies(c.Request.Context(), spaceID, 0)
	if err != nil {
		respondFusionError(c, err)
		return
	}
	dto.ResponseSuccess(c, toFusionStrategyList(strategies))
}

// Rollback activates a previous strategy version.
func (h *FusionHandler) Rollback(c *gin.Context) {
	if h == nil || h.svc == nil {
		c.Status(http.StatusNotImplemented)
		return
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(c.Param("spaceId")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "空间 ID 无效", err)
		return
	}
	strategyID, err := strconv.ParseUint(strings.TrimSpace(c.Param("strategyId")), 10, 64)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "策略版本 ID 无效", err)
		return
	}
	strategy, err := h.svc.RollbackStrategy(c.Request.Context(), ksvc.RollbackStrategyInput{
		SpaceID:    spaceID,
		StrategyID: strategyID,
	})
	if err != nil {
		respondFusionError(c, err)
		return
	}
	dto.ResponseSuccess(c, toFusionStrategyResponse(strategy))
}

func respondFusionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ksvc.ErrInvalidInput):
		dto.ResponseError(c, http.StatusBadRequest, "参数不合法", err)
	case errors.Is(err, ksvc.ErrSpaceNotFound):
		dto.ResponseError(c, http.StatusNotFound, "知识空间不存在或已退役", err)
	case errors.Is(err, ksvc.ErrFusionConflict):
		dto.ResponseError(c, http.StatusConflict, "存在激活策略，冲突策略被阻止", err)
	case errors.Is(err, ksvc.ErrFusionStrategyNotFound):
		dto.ResponseError(c, http.StatusNotFound, "策略版本不存在", err)
	default:
		dto.ResponseError(c, http.StatusInternalServerError, "融合策略操作失败", err)
	}
}
