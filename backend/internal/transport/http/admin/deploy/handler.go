package deploy

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	dtoops "github.com/ArtisanCloud/PowerX/internal/dto/ops"
	deployops "github.com/ArtisanCloud/PowerX/internal/service/deploy_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type handler struct {
	svc *deployops.Service
}

func NewHandler(deps *shared.Deps) *handler {
	if deps == nil || deps.DB == nil {
		return nil
	}
	return &handler{svc: deployops.NewService(deps.DB)}
}

func (h *handler) ListReleases(c *gin.Context) {
	environment := strings.TrimSpace(c.Query("environment"))
	page := parseInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parseInt(c.DefaultQuery("page_size", "20"), 20)

	items, total, err := h.svc.ListReleases(c.Request.Context(), deployops.ListReleaseOptions{
		Environment: environment,
		Page:        page,
		PageSize:    pageSize,
	})
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "list deploy releases failed", err)
		return
	}
	resp := gin.H{
		"items": items,
		"pagination": gin.H{
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	}
	dto.ResponseSuccess(c, resp)
}

func (h *handler) TriggerRelease(c *gin.Context) {
	var req dtoops.DeployReleaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}

	mode := strings.TrimSpace(c.Query("mode"))
	approvalTickets := parseInt(c.Query("approval_tickets"), 0)
	operator := resolveOperator(c)
	traceID := strings.TrimSpace(reqctx.GetTraceID(c.Request.Context()))

	row, err := h.svc.TriggerRelease(c.Request.Context(), deployops.ReleaseRequest{
		Environment:     req.Environment,
		BackendVersion:  req.BackendVersion,
		WebAdminVersion: req.WebAdminVer,
		Mode:            mode,
		Operator:        operator,
		TraceID:         traceID,
		ApprovalTickets: approvalTickets,
	})
	if err != nil {
		h.respondDeployError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"release": row})
}

func (h *handler) TriggerRollback(c *gin.Context) {
	var req dtoops.DeployRollbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}

	mode := strings.TrimSpace(c.Query("mode"))
	approvalTickets := parseInt(c.Query("approval_tickets"), 0)
	operator := resolveOperator(c)
	traceID := strings.TrimSpace(reqctx.GetTraceID(c.Request.Context()))

	row, err := h.svc.TriggerRollback(c.Request.Context(), deployops.RollbackRequest{
		Environment:     req.Environment,
		TargetVersion:   req.TargetVersion,
		Mode:            mode,
		Operator:        operator,
		TraceID:         traceID,
		ApprovalTickets: approvalTickets,
	})
	if err != nil {
		h.respondDeployError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"release": row})
}

func (h *handler) GetHealth(c *gin.Context) {
	health, err := h.svc.GetHealth(c.Request.Context())
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "get deploy health failed", err)
		return
	}
	dto.ResponseSuccess(c, health)
}

func (h *handler) respondDeployError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, deployops.ErrInvalidDeployRequest), errors.Is(err, deployops.ErrInvalidDeployMode):
		dto.ResponseError(c, http.StatusBadRequest, "invalid deploy request", err)
	case errors.Is(err, deployops.ErrReleaseInProgress):
		dto.ResponseError(c, http.StatusConflict, "release already in progress", err)
	case errors.Is(err, deployops.ErrApprovalRequired):
		dto.ResponseError(c, http.StatusForbidden, "approval required", err)
	default:
		dto.ResponseError(c, http.StatusInternalServerError, "deploy operation failed", err)
	}
}

func resolveOperator(c *gin.Context) string {
	ctx := c.Request.Context()
	if reqctx.IsRoot(ctx) {
		return "root"
	}
	if memberID := reqctx.GetMemberID(ctx); memberID > 0 {
		return "member"
	}
	return "system"
}

func parseInt(raw string, fallback int) int {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	v, _ := strconv.Atoi(strings.TrimSpace(raw))
	if v <= 0 {
		return fallback
	}
	return v
}
