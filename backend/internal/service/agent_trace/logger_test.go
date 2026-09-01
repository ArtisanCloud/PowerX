package agent_trace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoggerFailFastMissingContext(t *testing.T) {
	logger := NewLogger(Config{Enabled: true, LocalEnabled: true, LocalDir: t.TempDir()})
	_, err := logger.StartRun(context.Background(), AgentRunMeta{
		TraceID:    "trace-1",
		RunID:      "run-1",
		TenantUUID: "tenant-1",
		AgentID:    "agent-1",
		SessionID:  "session-1",
	})
	if err == nil {
		t.Fatal("expected missing message_id error")
	}
	var traceErr *TraceError
	if !errors.As(err, &traceErr) {
		t.Fatalf("expected TraceError, got %T", err)
	}
	if traceErr.Code != ErrCodeContextMissing {
		t.Fatalf("unexpected code: %s", traceErr.Code)
	}
}

func TestLocalSinkWritesRunTimelineNodesAndReport(t *testing.T) {
	root := t.TempDir()
	logger := NewLogger(Config{Enabled: true, LocalEnabled: true, LocalDir: root})
	ctx := context.Background()
	meta := AgentRunMeta{
		TraceID:           "trace-1",
		RunID:             "run-1",
		TenantUUID:        "tenant-1",
		UserUUID:          "user-1",
		AgentID:           "agent-1",
		SessionID:         "session-1",
		MessageID:         "message-1",
		Channel:           "web",
		UserMessageDigest: "sha256:user",
	}
	run, err := logger.StartRun(ctx, meta)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	start := time.Now().UTC()
	if err := logger.StartNode(ctx, AgentTraceNode{
		AgentRunMeta: meta,
		NodeID:       "node-1",
		NodeSeq:      1,
		NodeKind:     "intent_recognition",
		NodeRef:      "detect",
		InputSummary: map[string]any{"accepted": true},
		StartedAt:    start,
	}); err != nil {
		t.Fatalf("StartNode: %v", err)
	}
	if err := logger.EndNode(ctx, AgentTraceNodeResult{
		AgentRunMeta:  meta,
		NodeID:        "node-1",
		NodeSeq:       1,
		NodeKind:      "intent_recognition",
		NodeRef:       "detect",
		OutputSummary: map[string]any{"task_count": 1},
		StartedAt:     start,
		EndedAt:       start.Add(time.Millisecond),
	}); err != nil {
		t.Fatalf("EndNode: %v", err)
	}
	if err := logger.CompleteRun(ctx, AgentRunResult{
		AgentRunMeta:        meta,
		Status:              RunStatusCompleted,
		FinalResponseDigest: "sha256:final",
		StartedAt:           run.StartedAt,
		EndedAt:             start.Add(2 * time.Millisecond),
	}); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}
	base := filepath.Join(root, "tenant-1", "session-1", "message-1", "run-1")
	for _, rel := range []string{"run.json", "timeline.jsonl", "nodes/001_intent_recognition.json", "report.json", "report.md"} {
		if _, err := os.Stat(filepath.Join(base, rel)); err != nil {
			t.Fatalf("expected %s: %v", rel, err)
		}
	}
	reportRaw, err := os.ReadFile(filepath.Join(base, "report.json"))
	if err != nil {
		t.Fatalf("read persisted report: %v", err)
	}
	var persisted AgentRunReport
	if err := json.Unmarshal(reportRaw, &persisted); err != nil {
		t.Fatalf("decode persisted report: %v", err)
	}
	if persisted.Summary["status"] != RunStatusCompleted || persisted.Summary["node_count"] != float64(1) || persisted.Summary["event_count"] != float64(2) {
		t.Fatalf("persisted report was not refreshed on completion: %#v", persisted.Summary)
	}
	report, err := logger.BuildReport(ctx, AgentReportQuery{
		TenantUUID: "tenant-1",
		SessionID:  "session-1",
		MessageID:  "message-1",
		RunID:      "run-1",
	})
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}
	if report.RunID != "run-1" || len(report.Timeline) != 2 || len(report.Nodes) != 1 {
		t.Fatalf("unexpected report: run=%s timeline=%d nodes=%d", report.RunID, len(report.Timeline), len(report.Nodes))
	}
	if got := report.Nodes[0].InputSummary["accepted"]; got != true {
		t.Fatalf("expected merged input summary, got %#v", report.Nodes[0].InputSummary)
	}
	if got := report.Nodes[0].OutputSummary["task_count"]; got != float64(1) && got != 1 {
		t.Fatalf("expected merged output summary, got %#v", report.Nodes[0].OutputSummary)
	}
	sessionReport, err := NewLocalSink(root).BuildSessionReport(ctx, AgentReportQuery{
		TenantUUID: "tenant-1",
		SessionID:  "session-1",
	})
	if err != nil {
		t.Fatalf("BuildSessionReport: %v", err)
	}
	if sessionReport.ReportScope != "session" || sessionReport.Summary["message_count"] != 1 {
		t.Fatalf("unexpected session report: scope=%s summary=%#v", sessionReport.ReportScope, sessionReport.Summary)
	}
}

func TestLocalSinkPersistsRunStateSnapshot(t *testing.T) {
	root := t.TempDir()
	logger := NewLogger(Config{Enabled: true, LocalEnabled: true, LocalDir: root})
	ctx := context.Background()
	meta := AgentRunMeta{
		TraceID:    "trace-run-state",
		RunID:      "run-state",
		TenantUUID: "tenant-state",
		AgentID:    "agent-state",
		SessionID:  "session-state",
		MessageID:  "message-state",
	}
	if _, err := logger.StartRun(ctx, meta); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if err := logger.AppendRunStateEvent(ctx, meta, "agent_run.awaiting_params", map[string]any{
		"task_id":        "task-create",
		"node_kind":      "skill",
		"node_ref":       "powerxplugin.template.basic",
		"skill_id":       "powerxplugin.template.basic",
		"action":         "create",
		"missing_fields": []string{"template.title", "template.description"},
	}); err != nil {
		t.Fatalf("AppendRunStateEvent: %v", err)
	}
	if err := logger.CompleteRun(ctx, AgentRunResult{AgentRunMeta: meta, Status: RunStatusCompleted}); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "tenant-state", "session-state", "message-state", "run-state", "run_state.json")); err != nil {
		t.Fatalf("expected run_state.json: %v", err)
	}
	report, err := logger.BuildReport(ctx, AgentReportQuery{
		TenantUUID: meta.TenantUUID,
		SessionID:  meta.SessionID,
		MessageID:  meta.MessageID,
		RunID:      meta.RunID,
	})
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}
	if report.RunState == nil {
		t.Fatalf("missing run_state")
	}
	if len(report.RunState.PendingParams) != 1 {
		t.Fatalf("pending=%#v", report.RunState.PendingParams)
	}
	task := report.RunState.PendingParams[0]
	if task.TaskID != "task-create" || task.Status != "awaiting_params" || len(task.MissingFields) != 2 {
		t.Fatalf("bad task state: %#v", task)
	}
}

func TestLocalSinkSeparatesRepeatedRunsForSameMessage(t *testing.T) {
	root := t.TempDir()
	logger := NewLogger(Config{Enabled: true, LocalEnabled: true, LocalDir: root})
	ctx := context.Background()
	base := AgentRunMeta{
		TenantUUID: "tenant-repeat",
		AgentID:    "agent-repeat",
		SessionID:  "session-repeat",
		MessageID:  "message-repeat",
	}
	first := base
	first.RunID = "run-first"
	first.TraceID = "trace-first"
	second := base
	second.RunID = "run-second"
	second.TraceID = "trace-second"

	for _, meta := range []AgentRunMeta{first, second} {
		if _, err := logger.StartRun(ctx, meta); err != nil {
			t.Fatalf("start %s: %v", meta.RunID, err)
		}
		if err := logger.CompleteRun(ctx, AgentRunResult{AgentRunMeta: meta, Status: RunStatusCompleted}); err != nil {
			t.Fatalf("complete %s: %v", meta.RunID, err)
		}
	}

	for _, meta := range []AgentRunMeta{first, second} {
		report, err := logger.BuildReport(ctx, AgentReportQuery{
			TenantUUID: meta.TenantUUID,
			SessionID:  meta.SessionID,
			MessageID:  meta.MessageID,
			RunID:      meta.RunID,
		})
		if err != nil {
			t.Fatalf("build %s: %v", meta.RunID, err)
		}
		if report.RunID != meta.RunID || report.TraceID != meta.TraceID {
			t.Fatalf("report for %s mixed another run: %#v", meta.RunID, report)
		}
	}
}
