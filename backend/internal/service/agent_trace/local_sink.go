package agent_trace

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Sink interface {
	StartRun(ctx context.Context, run AgentRunTrace) error
	AppendEvent(ctx context.Context, event AgentTraceEvent) error
	WriteNodeSnapshot(ctx context.Context, meta AgentRunMeta, snapshot AgentTraceNodeSnapshot) error
	CompleteRun(ctx context.Context, run AgentRunTrace) error
}

type ReportSource interface {
	BuildReport(ctx context.Context, query AgentReportQuery) (*AgentRunReport, error)
	RenderMarkdown(report *AgentRunReport) string
}

type LocalSink struct {
	rootDir string
	mu      sync.Mutex
}

func NewLocalSink(rootDir string) *LocalSink {
	rootDir = strings.TrimSpace(rootDir)
	if rootDir == "" {
		rootDir = DefaultLocalDir
	}
	return &LocalSink{rootDir: rootDir}
}

func (s *LocalSink) StartRun(_ context.Context, run AgentRunTrace) error {
	if err := validateRunTrace(run); err != nil {
		return err
	}
	run.CreatedAt = defaultTime(run.CreatedAt)
	if run.Status == "" {
		run.Status = RunStatusRunning
	}
	dir := s.runDir(run.TenantUUID, run.SessionID, run.MessageID)
	if err := os.MkdirAll(filepath.Join(dir, "nodes"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "artifacts"), 0o755); err != nil {
		return err
	}
	run.ArtifactRoot = filepath.Join(dir, "artifacts")
	if err := writeJSONFile(filepath.Join(dir, "run.json"), run); err != nil {
		return err
	}
	report, err := s.BuildReport(context.Background(), AgentReportQuery{
		TenantUUID: run.TenantUUID,
		SessionID:  run.SessionID,
		MessageID:  run.MessageID,
		RunID:      run.RunID,
		TraceID:    run.TraceID,
		Source:     "local",
		Format:     "json",
	})
	if err != nil {
		return err
	}
	if err := writeJSONFile(filepath.Join(dir, "report.json"), report); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "report.md"), []byte(s.RenderMarkdown(report)), 0o644)
}

func (s *LocalSink) AppendEvent(_ context.Context, event AgentTraceEvent) error {
	if err := validateTraceEvent(event); err != nil {
		return err
	}
	event.CreatedAt = defaultTime(event.CreatedAt)
	dir := s.runDir(event.TenantUUID, event.SessionID, event.MessageID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	line, err := json.Marshal(event)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(filepath.Join(dir, "timeline.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	if _, err := w.Write(append(line, '\n')); err != nil {
		return err
	}
	return w.Flush()
}

func (s *LocalSink) WriteNodeSnapshot(_ context.Context, meta AgentRunMeta, snapshot AgentTraceNodeSnapshot) error {
	if err := validateRunMeta(meta); err != nil {
		return err
	}
	if strings.TrimSpace(snapshot.NodeID) == "" {
		return missingFieldsError("node_id")
	}
	if strings.TrimSpace(snapshot.NodeKind) == "" {
		return missingFieldsError("node_kind")
	}
	dir := filepath.Join(s.runDir(meta.TenantUUID, meta.SessionID, meta.MessageID), "nodes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := fmt.Sprintf("%03d_%s.json", snapshot.NodeSeq, safePathPart(snapshot.NodeKind))
	if snapshot.NodeSeq <= 0 {
		name = safePathPart(snapshot.NodeID) + ".json"
	}
	return writeJSONFile(filepath.Join(dir, name), snapshot)
}

func (s *LocalSink) CompleteRun(_ context.Context, run AgentRunTrace) error {
	if err := validateRunTrace(run); err != nil {
		return err
	}
	run.CreatedAt = defaultTime(run.CreatedAt)
	if run.EndedAt == nil {
		now := time.Now().UTC()
		run.EndedAt = &now
	}
	if run.StartedAt != nil && run.DurationMS <= 0 {
		run.DurationMS = run.EndedAt.Sub(*run.StartedAt).Milliseconds()
	}
	dir := s.runDir(run.TenantUUID, run.SessionID, run.MessageID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	run.ArtifactRoot = filepath.Join(dir, "artifacts")
	return writeJSONFile(filepath.Join(dir, "run.json"), run)
}

func (s *LocalSink) runDir(tenantUUID, sessionID, messageID string) string {
	return filepath.Join(s.rootDir, safePathPart(tenantUUID), safePathPart(sessionID), safePathPart(messageID))
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

var unsafePathPart = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func safePathPart(v string) string {
	v = strings.TrimSpace(v)
	v = strings.ReplaceAll(v, string(filepath.Separator), "_")
	v = unsafePathPart.ReplaceAllString(v, "_")
	v = strings.Trim(v, "._-")
	if v == "" {
		return "unknown"
	}
	if len(v) > 160 {
		return v[:160]
	}
	return v
}

func defaultTime(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}
