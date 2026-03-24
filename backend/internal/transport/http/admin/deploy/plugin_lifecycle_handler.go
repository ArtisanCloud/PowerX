package deploy

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	dtoops "github.com/ArtisanCloud/PowerX/internal/dto/ops"
	deployops "github.com/ArtisanCloud/PowerX/internal/service/deploy_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type pluginLifecycleHandler struct {
	svc *deployops.PluginLifecycleService
}

func NewPluginLifecycleHandler(deps *shared.Deps) *pluginLifecycleHandler {
	if deps == nil || deps.DB == nil {
		return nil
	}
	return &pluginLifecycleHandler{svc: deployops.NewPluginLifecycleService(deps.DB)}
}

func (h *pluginLifecycleHandler) ListPluginLifecycleAudits(c *gin.Context) {
	pluginID := strings.TrimSpace(c.Param("pluginId"))
	page := parseInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parseInt(c.DefaultQuery("page_size", "20"), 20)

	items, total, err := h.svc.ListAudits(c.Request.Context(), deployops.PluginLifecycleListOptions{
		PluginID: pluginID,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		h.respondError(c, err)
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

func (h *pluginLifecycleHandler) TriggerPluginLifecycleAction(c *gin.Context) {
	pluginID := strings.TrimSpace(c.Param("pluginId"))
	var req dtoops.PluginLifecycleActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	if strings.TrimSpace(req.PluginID) == "" {
		req.PluginID = pluginID
	}

	row, err := h.svc.TriggerAction(c.Request.Context(), deployops.PluginLifecycleActionRequest{
		PluginID:    req.PluginID,
		FromVersion: req.FromVersion,
		ToVersion:   req.ToVersion,
		Action:      req.Action,
		Reason:      req.Reason,
		Operator:    resolveOperator(c),
		TraceID:     strings.TrimSpace(reqctx.GetTraceID(c.Request.Context())),
	})
	if err != nil {
		h.respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"audit": row})
}

func (h *pluginLifecycleHandler) respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, deployops.ErrInvalidPluginLifecycleRequest), errors.Is(err, deployops.ErrUnsupportedPluginAction):
		dto.ResponseError(c, http.StatusBadRequest, "invalid plugin lifecycle request", err)
	default:
		dto.ResponseError(c, http.StatusInternalServerError, "plugin lifecycle operation failed", err)
	}
}
