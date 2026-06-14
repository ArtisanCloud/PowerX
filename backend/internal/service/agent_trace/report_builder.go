package agent_trace

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (l *Logger) BuildReport(ctx context.Context, query AgentReportQuery) (*AgentRunReport, error) {
	if l == nil || !l.cfg.Enabled {
		return nil, &TraceError{Code: ErrCodeSinkUnavailable, Message: "agent trace logger is disabled"}
	}
	query = normalizeReportQuery(query)
	if err := validateReportQuery(query); err != nil {
		return nil, err
	}
	for _, sink := range l.sinks {
		source, ok := sink.(ReportSource)
		if !ok || source == nil {
			continue
		}
		return source.BuildReport(ctx, query)
	}
	return nil, &TraceError{Code: ErrCodeReportUnsupported, Message: "no agent trace sink supports report building"}
}

func (s *LocalSink) BuildReport(_ context.Context, query AgentReportQuery) (*AgentRunReport, error) {
	query = normalizeReportQuery(query)
	if err := validateReportQuery(query); err != nil {
		return nil, err
	}
	dir := s.runDir(query.TenantUUID, query.SessionID, query.MessageID)
	run, err := readJSONFile[AgentRunTrace](filepath.Join(dir, "run.json"))
	if err != nil {
		return nil, err
	}
	timeline, err := readTimeline(filepath.Join(dir, "timeline.jsonl"))
	if err != nil {
		return nil, err
	}
	nodes, err := readNodes(filepath.Join(dir, "nodes"))
	if err != nil {
		return nil, err
	}
	errors := make([]map[string]any, 0)
	artifacts := map[string]struct{}{}
	for _, event := range timeline {
		if event.Status == EventStatusError || event.Phase == EventPhaseError {
			errors = append(errors, map[string]any{
				"node_id":       event.NodeID,
				"node_kind":     event.NodeKind,
				"error_code":    event.ErrorCode,
				"error_summary": event.ErrorSummary,
				"created_at":    event.CreatedAt,
			})
		}
		for _, ref := range event.ArtifactRefs {
			if strings.TrimSpace(ref) != "" {
				artifacts[ref] = struct{}{}
			}
		}
	}
	for _, node := range nodes {
		for _, ref := range node.ArtifactRefs {
			if strings.TrimSpace(ref) != "" {
				artifacts[ref] = struct{}{}
			}
		}
	}
	artifactRefs := make([]string, 0, len(artifacts))
	for ref := range artifacts {
		artifactRefs = append(artifactRefs, ref)
	}
	sort.Strings(artifactRefs)
	report := &AgentRunReport{
		ReportScope: "message",
		Format:      firstNonEmpty(query.Format, "json"),
		TenantUUID:  run.TenantUUID,
		SessionID:   run.SessionID,
		MessageID:   run.MessageID,
		RunID:       run.RunID,
		TraceID:     run.TraceID,
		GeneratedBy: "powerx-agent-trace-local",
		GeneratedAt: time.Now().UTC(),
		Summary: map[string]any{
			"status":                run.Status,
			"agent_id":              run.AgentID,
			"channel":               run.Channel,
			"node_count":            maxInt(run.NodeCount, len(nodes)),
			"event_count":           maxInt(run.EventCount, len(timeline)),
			"error_count":           maxInt(run.ErrorCount, len(errors)),
			"warning_count":         run.WarningCount,
			"duration_ms":           run.DurationMS,
			"user_message_digest":   run.UserMessageDigest,
			"final_response_digest": run.FinalResponseDigest,
			"started_at":            run.StartedAt,
			"ended_at":              run.EndedAt,
		},
		Timeline:     timeline,
		Nodes:        nodes,
		Errors:       errors,
		ArtifactRefs: artifactRefs,
	}
	return report, nil
}

func (s *LocalSink) RenderMarkdown(report *AgentRunReport) string {
	if report == nil {
		return "# Agent Run Report\n\n报告为空。\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Agent Run Report\n\n")
	fmt.Fprintf(&b, "- Tenant: `%s`\n", report.TenantUUID)
	fmt.Fprintf(&b, "- Session: `%s`\n", report.SessionID)
	fmt.Fprintf(&b, "- Message: `%s`\n", report.MessageID)
	fmt.Fprintf(&b, "- Run: `%s`\n", report.RunID)
	fmt.Fprintf(&b, "- Trace: `%s`\n", report.TraceID)
	fmt.Fprintf(&b, "- Generated At: `%s`\n\n", report.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "## Summary\n\n")
	for _, key := range []string{"status", "agent_id", "channel", "node_count", "event_count", "error_count", "duration_ms"} {
		if v, ok := report.Summary[key]; ok {
			fmt.Fprintf(&b, "- %s: `%v`\n", key, v)
		}
	}
	fmt.Fprintf(&b, "\n## Timeline\n\n")
	fmt.Fprintf(&b, "| Time | Node | Phase | Status | Duration |\n| --- | --- | --- | --- | --- |\n")
	for _, event := range report.Timeline {
		fmt.Fprintf(&b, "| %s | `%s` %s | %s | %s | %dms |\n",
			event.CreatedAt.Format(time.RFC3339), event.NodeKind, event.NodeID, event.Phase, event.Status, event.DurationMS)
	}
	if len(report.Errors) > 0 {
		fmt.Fprintf(&b, "\n## Errors\n\n")
		for _, item := range report.Errors {
			fmt.Fprintf(&b, "- `%v` %v: %v\n", item["node_id"], item["error_code"], item["error_summary"])
		}
	}
	return b.String()
}

func normalizeReportQuery(query AgentReportQuery) AgentReportQuery {
	query.TenantUUID = strings.TrimSpace(query.TenantUUID)
	query.SessionID = strings.TrimSpace(query.SessionID)
	query.MessageID = strings.TrimSpace(query.MessageID)
	query.RunID = strings.TrimSpace(query.RunID)
	query.TraceID = strings.TrimSpace(query.TraceID)
	query.Source = strings.TrimSpace(query.Source)
	query.Format = strings.TrimSpace(query.Format)
	if query.Source == "" {
		query.Source = "local"
	}
	if query.Format == "" {
		query.Format = "json"
	}
	return query
}

func validateReportQuery(query AgentReportQuery) error {
	return validateRequired(map[string]string{
		"tenant_uuid": query.TenantUUID,
		"session_id":  query.SessionID,
		"message_id":  query.MessageID,
	})
}

func readJSONFile[T any](path string) (T, error) {
	var out T
	data, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

func readTimeline(path string) ([]AgentTraceEvent, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []AgentTraceEvent{}, nil
		}
		return nil, err
	}
	defer file.Close()
	out := []AgentTraceEvent{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event AgentTraceEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].NodeSeq < out[j].NodeSeq
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func readNodes(dir string) ([]AgentTraceNodeSnapshot, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []AgentTraceNodeSnapshot{}, nil
		}
		return nil, err
	}
	out := []AgentTraceNodeSnapshot{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		node, err := readJSONFile[AgentTraceNodeSnapshot](filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, node)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].NodeSeq == out[j].NodeSeq {
			return out[i].NodeID < out[j].NodeID
		}
		return out[i].NodeSeq < out[j].NodeSeq
	})
	return out, nil
}
