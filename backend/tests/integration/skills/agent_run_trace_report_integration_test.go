package skillsintegration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_trace"
	agenttraceapi "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/agenttrace"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAgentRunTraceReportQueryByMessageID(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AGENT_TRACE_ENABLED", "true")
	t.Setenv("AGENT_TRACE_LOCAL_ENABLED", "true")
	t.Setenv("AGENT_TRACE_LOCAL_DIR", root)

	ctx := context.Background()
	logger := agent_trace.NewLogger(agent_trace.Config{Enabled: true, LocalEnabled: true, LocalDir: root})
	meta := agent_trace.AgentRunMeta{
		TraceID:    "trace-integration-1",
		RunID:      "run-integration-1",
		TenantUUID: "tenant-integration",
		AgentID:    "agent-integration",
		SessionID:  "session-integration",
		MessageID:  "message-integration",
	}
	runCtx, err := logger.StartRun(ctx, meta)
	require.NoError(t, err)
	start := time.Now().UTC()
	require.NoError(t, logger.StartNode(ctx, agent_trace.AgentTraceNode{
		AgentRunMeta: meta,
		NodeID:       "001_receive_message",
		NodeSeq:      1,
		NodeKind:     "receive_message",
		StartedAt:    start,
	}))
	require.NoError(t, logger.EndNode(ctx, agent_trace.AgentTraceNodeResult{
		AgentRunMeta: meta,
		NodeID:       "001_receive_message",
		NodeSeq:      1,
		NodeKind:     "receive_message",
		StartedAt:    start,
		EndedAt:      start.Add(time.Millisecond),
	}))
	require.NoError(t, logger.CompleteRun(ctx, agent_trace.AgentRunResult{
		AgentRunMeta: meta,
		Status:       agent_trace.RunStatusCompleted,
		StartedAt:    runCtx.StartedAt,
		EndedAt:      start.Add(2 * time.Millisecond),
	}))

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(reqctx.WithIsRoot(c.Request.Context(), true))
		c.Next()
	})
	group := router.Group("/api/v1")
	agenttraceapi.RegisterAPIRoutes(nil, group, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/agent-traces/messages/message-integration/timeline?tenant_uuid=tenant-integration&session_id=session-integration", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "001_receive_message")

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/agent-traces/messages/message-integration/report?tenant_uuid=tenant-integration&session_id=session-integration&format=markdown", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.True(t, strings.Contains(rec.Body.String(), "Agent Run Report"))
}
