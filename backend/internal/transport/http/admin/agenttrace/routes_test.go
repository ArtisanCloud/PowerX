package agenttrace

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_trace"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
)

func TestAgentTraceRootOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1")
	RegisterAPIRoutes(nil, group, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/agent-traces/messages/message-1?tenant_uuid=tenant-1&session_id=session-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["message"] != "AGENT_TRACE_ROOT_REQUIRED" {
		t.Fatalf("message=%v", body["message"])
	}
}

func TestAgentTraceReportAndTimelineAPI(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AGENT_TRACE_LOCAL_DIR", root)
	t.Setenv("AGENT_TRACE_LOCAL_ENABLED", "true")
	t.Setenv("AGENT_TRACE_ENABLED", "true")

	logger := agent_trace.NewLogger(agent_trace.Config{Enabled: true, LocalEnabled: true, LocalDir: root})
	meta := agent_trace.AgentRunMeta{
		TraceID:    "trace-1",
		RunID:      "run-1",
		TenantUUID: "tenant-1",
		AgentID:    "agent-1",
		SessionID:  "session-1",
		MessageID:  "message-1",
	}
	ctx := context.Background()
	run, err := logger.StartRun(ctx, meta)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	start := time.Now().UTC()
	if err := logger.StartNode(ctx, agent_trace.AgentTraceNode{AgentRunMeta: meta, NodeID: "node-1", NodeSeq: 1, NodeKind: "receive_message", StartedAt: start}); err != nil {
		t.Fatalf("StartNode: %v", err)
	}
	if err := logger.EndNode(ctx, agent_trace.AgentTraceNodeResult{AgentRunMeta: meta, NodeID: "node-1", NodeSeq: 1, NodeKind: "receive_message", StartedAt: start, EndedAt: start.Add(time.Millisecond)}); err != nil {
		t.Fatalf("EndNode: %v", err)
	}
	if err := logger.CompleteRun(ctx, agent_trace.AgentRunResult{AgentRunMeta: meta, Status: agent_trace.RunStatusCompleted, StartedAt: run.StartedAt, EndedAt: start.Add(2 * time.Millisecond)}); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}

	if _, err := filepath.Abs(root); err != nil {
		t.Fatalf("invalid temp dir: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(reqctx.WithIsRoot(c.Request.Context(), true))
		c.Next()
	})
	group := router.Group("/api/v1")
	RegisterAPIRoutes(nil, group, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/agent-traces/messages/message-1/timeline?tenant_uuid=tenant-1&session_id=session-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("timeline status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !json.Valid(rec.Body.Bytes()) {
		t.Fatalf("timeline body is not json: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/agent-traces/runs?tenant_uuid=tenant-1", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("runs status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !json.Valid(rec.Body.Bytes()) {
		t.Fatalf("runs body is not json: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/agent-traces/messages/message-1/report?tenant_uuid=tenant-1&session_id=session-1&format=markdown", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("report status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/markdown; charset=utf-8" {
		t.Fatalf("content-type=%q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/agent-traces/sessions/session-1/report?tenant_uuid=tenant-1&format=markdown", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("session report status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/markdown; charset=utf-8" {
		t.Fatalf("session report content-type=%q", got)
	}
}
