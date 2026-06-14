package agenttrace

import (
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	agenttrace "github.com/ArtisanCloud/PowerX/internal/service/agent_trace"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func RegisterAPIRoutes(_ *gin.RouterGroup, protected *gin.RouterGroup, _ *shared.Deps) {
	if protected == nil {
		return
	}
	h := &handler{logger: agenttrace.NewLoggerFromEnv()}
	group := protected.Group("/admin/agent-traces")
	group.Use(rootOnly())
	group.GET("/messages/:message_id", h.message)
	group.GET("/messages/:message_id/timeline", h.timeline)
	group.GET("/messages/:message_id/report", h.report)
	group.GET("/sessions/:session_id/report", h.sessionReport)
}

type handler struct {
	logger agenttrace.AgentTraceLogger
}

func rootOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !reqctx.IsRoot(c.Request.Context()) {
			dto.ResponseError(c, http.StatusForbidden, "AGENT_TRACE_ROOT_REQUIRED", nil)
			c.Abort()
			return
		}
		c.Next()
	}
}

func (h *handler) message(c *gin.Context) {
	report, err := h.build(c, c.Param("message_id"))
	if err != nil {
		respondTraceError(c, err)
		return
	}
	dto.ResponseSuccess(c, map[string]any{
		"tenant_uuid": report.TenantUUID,
		"session_id":  report.SessionID,
		"message_id":  report.MessageID,
		"run_id":      report.RunID,
		"trace_id":    report.TraceID,
		"summary":     report.Summary,
		"node_count":  len(report.Nodes),
		"event_count": len(report.Timeline),
		"error_count": len(report.Errors),
	})
}

func (h *handler) timeline(c *gin.Context) {
	report, err := h.build(c, c.Param("message_id"))
	if err != nil {
		respondTraceError(c, err)
		return
	}
	dto.ResponseSuccess(c, map[string]any{
		"items":       report.Timeline,
		"nodes":       report.Nodes,
		"tenant_uuid": report.TenantUUID,
		"session_id":  report.SessionID,
		"message_id":  report.MessageID,
	})
}

func (h *handler) report(c *gin.Context) {
	report, err := h.build(c, c.Param("message_id"))
	if err != nil {
		respondTraceError(c, err)
		return
	}
	if strings.EqualFold(c.Query("format"), "markdown") || strings.EqualFold(c.Query("download"), "md") {
		md := (&agenttrace.LocalSink{}).RenderMarkdown(report)
		c.Header("Content-Disposition", `attachment; filename="agent-run-report.md"`)
		c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(md))
		return
	}
	if strings.EqualFold(c.Query("download"), "json") {
		c.Header("Content-Disposition", `attachment; filename="agent-run-report.json"`)
	}
	dto.ResponseSuccess(c, report)
}

func (h *handler) sessionReport(c *gin.Context) {
	messageID := strings.TrimSpace(c.Query("message_id"))
	if messageID == "" {
		respondTraceError(c, &agenttrace.TraceError{Code: agenttrace.ErrCodeContextMissing, Message: "message_id query is required", MissingFields: []string{"message_id"}})
		return
	}
	report, err := h.build(c, messageID)
	if err != nil {
		respondTraceError(c, err)
		return
	}
	dto.ResponseSuccess(c, report)
}

func (h *handler) build(c *gin.Context, messageID string) (*agenttrace.AgentRunReport, error) {
	tenantUUID := strings.TrimSpace(c.Query("tenant_uuid"))
	if tenantUUID == "" {
		tenantUUID = strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	}
	query := agenttrace.AgentReportQuery{
		TenantUUID: tenantUUID,
		SessionID:  firstQuery(c, "session_id", "sessionId"),
		MessageID:  strings.TrimSpace(messageID),
		RunID:      strings.TrimSpace(c.Query("run_id")),
		TraceID:    strings.TrimSpace(c.Query("trace_id")),
		Source:     firstQuery(c, "source"),
		Format:     firstQuery(c, "format"),
	}
	return h.logger.BuildReport(c.Request.Context(), query)
}

func respondTraceError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	message := "AGENT_TRACE_INTERNAL"
	if te, ok := err.(*agenttrace.TraceError); ok {
		message = te.Code
		switch te.Code {
		case agenttrace.ErrCodeContextMissing:
			status = http.StatusBadRequest
		case agenttrace.ErrCodeReportUnsupported, agenttrace.ErrCodeSinkUnavailable:
			status = http.StatusServiceUnavailable
		}
	}
	dto.ResponseError(c, status, message, err)
}

func firstQuery(c *gin.Context, keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(c.Query(key)); v != "" {
			return v
		}
	}
	return ""
}
