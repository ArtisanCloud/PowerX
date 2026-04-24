package monitor

import (
	"net/http"
	"strings"

	monitorlogs "github.com/ArtisanCloud/PowerX/internal/service/monitor_logs"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func (h *handler) ListPluginLoggingTargets(c *gin.Context) {
	traceID := strings.TrimSpace(reqctx.GetTraceID(c.Request.Context()))
	operator := resolveOperator(c)
	items, err := h.svc.ListEnabledPluginRuntimeTargets(c.Request.Context())
	if err != nil {
		logger.Info(c.Request.Context(), "monitor.logs.plugins.list",
			zap.String("operator", operator),
			zap.String("trace_id", traceID),
			zap.String("status", "failed"),
			zap.String("error", err.Error()),
		)
		dto.ResponseError(c, http.StatusInternalServerError, "list plugin logging targets failed", err)
		return
	}
	logger.Info(c.Request.Context(), "monitor.logs.plugins.list",
		zap.String("operator", operator),
		zap.String("trace_id", traceID),
		zap.String("status", "success"),
		zap.Int("count", len(items)),
	)
	dto.ResponseSuccess(c, gin.H{"items": items})
}

func (h *handler) GetPluginLoggingPolicy(c *gin.Context) {
	pluginID := strings.TrimSpace(c.Param("id"))
	if pluginID == "" {
		dto.ResponseError(c, http.StatusBadRequest, "plugin_id is required", nil)
		return
	}
	traceID := strings.TrimSpace(reqctx.GetTraceID(c.Request.Context()))
	operator := resolveOperator(c)
	out, err := h.svc.GetPluginLoggingPolicy(c.Request.Context(), buildPluginLoggingRequest(c, pluginID, nil))
	if err != nil {
		logger.Info(c.Request.Context(), "monitor.logs.plugins.policy.get",
			zap.String("operator", operator),
			zap.String("trace_id", traceID),
			zap.String("plugin_id", pluginID),
			zap.String("status", "failed"),
			zap.String("error", err.Error()),
		)
		dto.ResponseError(c, http.StatusBadGateway, "get plugin logging policy failed", err)
		return
	}
	logger.Info(c.Request.Context(), "monitor.logs.plugins.policy.get",
		zap.String("operator", operator),
		zap.String("trace_id", traceID),
		zap.String("plugin_id", pluginID),
		zap.String("status", "success"),
	)
	dto.ResponseSuccess(c, out)
}

func (h *handler) PutPluginLoggingPolicy(c *gin.Context) {
	pluginID := strings.TrimSpace(c.Param("id"))
	if pluginID == "" {
		dto.ResponseError(c, http.StatusBadRequest, "plugin_id is required", nil)
		return
	}
	payload, ok := parseJSONBodyAsMap(c)
	if !ok {
		return
	}

	traceID := strings.TrimSpace(reqctx.GetTraceID(c.Request.Context()))
	operator := resolveOperator(c)
	out, err := h.svc.UpdatePluginLoggingPolicy(c.Request.Context(), buildPluginLoggingRequest(c, pluginID, payload))
	if err != nil {
		logger.Info(c.Request.Context(), "monitor.logs.plugins.policy.put",
			zap.String("operator", operator),
			zap.String("trace_id", traceID),
			zap.String("plugin_id", pluginID),
			zap.String("status", "failed"),
			zap.String("error", err.Error()),
		)
		dto.ResponseError(c, http.StatusBadGateway, "update plugin logging policy failed", err)
		return
	}
	logger.Info(c.Request.Context(), "monitor.logs.plugins.policy.put",
		zap.String("operator", operator),
		zap.String("trace_id", traceID),
		zap.String("plugin_id", pluginID),
		zap.String("status", "success"),
	)
	dto.ResponseSuccess(c, out)
}

func (h *handler) ProbePluginLoggingPolicy(c *gin.Context) {
	pluginID := strings.TrimSpace(c.Param("id"))
	if pluginID == "" {
		dto.ResponseError(c, http.StatusBadRequest, "plugin_id is required", nil)
		return
	}
	payload, ok := parseJSONBodyAsMap(c)
	if !ok {
		return
	}

	traceID := strings.TrimSpace(reqctx.GetTraceID(c.Request.Context()))
	operator := resolveOperator(c)
	out, err := h.svc.ProbePluginLoggingPolicy(c.Request.Context(), buildPluginLoggingRequest(c, pluginID, payload))
	if err != nil {
		logger.Info(c.Request.Context(), "monitor.logs.plugins.probe",
			zap.String("operator", operator),
			zap.String("trace_id", traceID),
			zap.String("plugin_id", pluginID),
			zap.String("status", "failed"),
			zap.String("error", err.Error()),
		)
		dto.ResponseError(c, http.StatusBadGateway, "probe plugin logging policy failed", err)
		return
	}
	logger.Info(c.Request.Context(), "monitor.logs.plugins.probe",
		zap.String("operator", operator),
		zap.String("trace_id", traceID),
		zap.String("plugin_id", pluginID),
		zap.String("status", "success"),
	)
	dto.ResponseSuccess(c, out)
}

func buildPluginLoggingRequest(c *gin.Context, pluginID string, payload map[string]any) monitorlogs.PluginLoggingRequest {
	return monitorlogs.PluginLoggingRequest{
		PluginID:      pluginID,
		BaseURL:       "",
		Authorization: strings.TrimSpace(c.GetHeader("Authorization")),
		RequestID:     strings.TrimSpace(c.GetHeader("X-Request-ID")),
		Body:          payload,
	}
}

func parseJSONBodyAsMap(c *gin.Context) (map[string]any, bool) {
	var payload map[string]any
	if err := c.ShouldBindJSON(&payload); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid payload", err)
		return nil, false
	}
	if payload == nil {
		payload = map[string]any{}
	}
	return payload, true
}
