package agenttrace

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agenttrace "github.com/ArtisanCloud/PowerX/internal/service/agent_trace"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func RegisterAPIRoutes(_ *gin.RouterGroup, protected *gin.RouterGroup, deps *shared.Deps) {
	if protected == nil {
		return
	}
	var db *gorm.DB
	if deps != nil {
		db = deps.DB
	}
	h := &handler{logger: agenttrace.NewLoggerFromEnv(), db: db}
	group := protected.Group("/admin/agent-traces")
	group.Use(rootOnly())
	group.GET("/runs", h.runs)
	group.GET("/sessions", h.sessions)
	group.GET("/messages/:message_id", h.message)
	group.GET("/messages/:message_id/timeline", h.timeline)
	group.GET("/messages/:message_id/report", h.report)
	group.GET("/sessions/:session_id/report", h.sessionReport)
}

type handler struct {
	logger agenttrace.AgentTraceLogger
	db     *gorm.DB
}

func (h *handler) runs(c *gin.Context) {
	tenantUUID := strings.TrimSpace(c.Query("tenant_uuid"))
	if tenantUUID == "" {
		tenantUUID = strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	}
	traceCfg := agenttrace.ConfigFromEnv()
	source := agenttrace.NewLocalSink(traceCfg.LocalDir)
	result, err := source.ListRuns(c.Request.Context(), agenttrace.AgentRunListQuery{
		TenantUUID: tenantUUID,
		SessionID:  strings.TrimSpace(c.Query("session_id")),
		Status:     strings.TrimSpace(c.Query("status")),
		Offset:     parsePositiveInt(c.Query("offset"), 0),
		Limit:      parsePositiveInt(c.Query("limit"), 50),
	})
	if err != nil {
		respondTraceError(c, err)
		return
	}
	h.attachMessagePreviews(c.Request.Context(), tenantUUID, result.Items)
	if strings.TrimSpace(c.Query("session_id")) != "" && result.Total == 0 {
		h.attachUntracedSessionMessages(c.Request.Context(), tenantUUID, strings.TrimSpace(c.Query("session_id")), &result)
	}
	dto.ResponseSuccess(c, result)
}

func (h *handler) sessions(c *gin.Context) {
	tenantUUID := strings.TrimSpace(c.Query("tenant_uuid"))
	if tenantUUID == "" {
		tenantUUID = strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	}
	traceCfg := agenttrace.ConfigFromEnv()
	source := agenttrace.NewLocalSink(traceCfg.LocalDir)
	result, err := source.ListSessions(c.Request.Context(), agenttrace.AgentRunListQuery{
		TenantUUID: tenantUUID,
		Status:     strings.TrimSpace(c.Query("status")),
		Offset:     parsePositiveInt(c.Query("offset"), 0),
		Limit:      parsePositiveInt(c.Query("limit"), 50),
	})
	if err != nil {
		respondTraceError(c, err)
		return
	}
	dto.ResponseSuccess(c, result)
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
		"run_state":   report.RunState,
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
		"run_state":   report.RunState,
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
	tenantUUID := strings.TrimSpace(c.Query("tenant_uuid"))
	if tenantUUID == "" {
		tenantUUID = strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	}
	query := agenttrace.AgentReportQuery{
		TenantUUID: tenantUUID,
		SessionID:  strings.TrimSpace(c.Param("session_id")),
		Source:     firstQuery(c, "source"),
		Format:     firstQuery(c, "format"),
	}
	traceCfg := agenttrace.ConfigFromEnv()
	source := agenttrace.NewLocalSink(traceCfg.LocalDir)
	report, err := source.BuildSessionReport(c.Request.Context(), query)
	if err != nil {
		respondTraceError(c, err)
		return
	}
	if strings.EqualFold(c.Query("format"), "markdown") || strings.EqualFold(c.Query("download"), "md") {
		md := source.RenderMarkdown(report)
		c.Header("Content-Disposition", `attachment; filename="agent-session-report.md"`)
		c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(md))
		return
	}
	if strings.EqualFold(c.Query("download"), "json") {
		c.Header("Content-Disposition", `attachment; filename="agent-session-report.json"`)
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

func parsePositiveInt(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	n := 0
	for _, r := range raw {
		if r < '0' || r > '9' {
			return fallback
		}
		n = n*10 + int(r-'0')
	}
	if n <= 0 {
		return fallback
	}
	return n
}

func (h *handler) attachMessagePreviews(ctx context.Context, tenantUUID string, items []agenttrace.AgentRunListItem) {
	if h == nil || h.db == nil || len(items) == 0 {
		return
	}
	ids := make([]uint64, 0, len(items))
	seen := map[uint64]struct{}{}
	for _, item := range items {
		id, err := strconv.ParseUint(strings.TrimSpace(item.MessageID), 10, 64)
		if err != nil || id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return
	}
	var tenantRef *string
	if strings.TrimSpace(tenantUUID) != "" {
		v := strings.TrimSpace(tenantUUID)
		tenantRef = &v
	}
	env := strings.TrimSpace(reqctx.GetEnv(ctx))
	if env == "" {
		env = "dev"
	}
	var messages []agentmodel.AgentChatMessage
	if err := h.db.WithContext(ctx).
		Scopes(agentmodel.WithScope(env, tenantRef)).
		Where("id IN ?", ids).
		Find(&messages).Error; err != nil {
		return
	}
	byID := make(map[string]agentmodel.AgentChatMessage, len(messages))
	for _, msg := range messages {
		byID[strconv.FormatUint(msg.ID, 10)] = msg
	}
	for idx := range items {
		msg, ok := byID[strings.TrimSpace(items[idx].MessageID)]
		if !ok {
			continue
		}
		items[idx].MessageRole = strings.TrimSpace(msg.Role)
		items[idx].MessagePreview = messagePreview(msg.Content, 120)
		createdAt := msg.CreatedAt
		items[idx].MessageCreatedAt = &createdAt
	}
}

func messagePreview(content string, limit int) string {
	content = strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	if content == "" {
		return ""
	}
	runes := []rune(content)
	if limit <= 0 || len(runes) <= limit {
		return content
	}
	return string(runes[:limit]) + "..."
}

func (h *handler) attachUntracedSessionMessages(ctx context.Context, tenantUUID, sessionID string, result *agenttrace.AgentRunListResult) {
	if h == nil || h.db == nil || result == nil {
		return
	}
	parsedSessionID, err := strconv.ParseUint(strings.TrimSpace(sessionID), 10, 64)
	if err != nil || parsedSessionID == 0 {
		return
	}
	var tenantRef *string
	if strings.TrimSpace(tenantUUID) != "" {
		v := strings.TrimSpace(tenantUUID)
		tenantRef = &v
	}
	env := strings.TrimSpace(reqctx.GetEnv(ctx))
	if env == "" {
		env = "dev"
	}
	var messages []agentmodel.AgentChatMessage
	if err := h.db.WithContext(ctx).
		Scopes(agentmodel.WithScope(env, tenantRef)).
		Where("session_id = ?", parsedSessionID).
		Order("id ASC").
		Find(&messages).Error; err != nil || len(messages) == 0 {
		return
	}
	items := make([]agenttrace.AgentRunListItem, 0, len(messages))
	for _, msg := range messages {
		status := agenttrace.RunStatusCompleted
		if msg.IsError || strings.EqualFold(strings.TrimSpace(msg.Role), "assistant") && strings.Contains(strings.ToLower(msg.Content), "执行失败") {
			status = agenttrace.RunStatusFailed
		}
		createdAt := msg.CreatedAt
		items = append(items, agenttrace.AgentRunListItem{
			TenantUUID:       tenantUUID,
			SessionID:        sessionID,
			MessageID:        strconv.FormatUint(msg.ID, 10),
			MessagePreview:   messagePreview(msg.Content, 120),
			MessageRole:      strings.TrimSpace(msg.Role),
			MessageCreatedAt: &createdAt,
			RunID:            "",
			TraceID:          "",
			AgentID:          strconv.FormatUint(msg.AgentID, 10),
			Status:           status,
			NodeCount:        0,
			EventCount:       0,
			ErrorCount:       boolToInt(status == agenttrace.RunStatusFailed),
			DurationMS:       0,
			CreatedAt:        msg.CreatedAt,
		})
	}
	result.Items = items
	result.Total = len(items)
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
