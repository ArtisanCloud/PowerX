package agent_trace

import (
	"context"
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
	base := filepath.Join(root, "tenant-1", "session-1", "message-1")
	for _, rel := range []string{"run.json", "timeline.jsonl", "nodes/001_intent_recognition.json", "report.json", "report.md"} {
		if _, err := os.Stat(filepath.Join(base, rel)); err != nil {
			t.Fatalf("expected %s: %v", rel, err)
		}
	}
	report, err := logger.BuildReport(ctx, AgentReportQuery{
		TenantUUID: "tenant-1",
		SessionID:  "session-1",
		MessageID:  "message-1",
	})
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}
	if report.RunID != "run-1" || len(report.Timeline) != 2 || len(report.Nodes) != 1 {
		t.Fatalf("unexpected report: run=%s timeline=%d nodes=%d", report.RunID, len(report.Timeline), len(report.Nodes))
	}
}
