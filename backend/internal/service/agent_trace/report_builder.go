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
	runState, _ := readJSONFile[AgentRunStateSnapshot](filepath.Join(dir, "run_state.json"))
	var runStatePtr *AgentRunStateSnapshot
	if len(runState.Tasks) > 0 || len(runState.PendingParams) > 0 || len(runState.Results) > 0 || len(runState.Errors) > 0 || runState.ResponsePlan != nil {
		runStatePtr = &runState
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
		RunState:     runStatePtr,
		Errors:       errors,
		ArtifactRefs: artifactRefs,
	}
	return report, nil
}

func (s *LocalSink) BuildSessionReport(_ context.Context, query AgentReportQuery) (*AgentRunReport, error) {
	query = normalizeReportQuery(query)
	if err := validateSessionReportQuery(query); err != nil {
		return nil, err
	}
	sessionDir := filepath.Join(s.rootDir, safePathPart(query.TenantUUID), safePathPart(query.SessionID))
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		return nil, err
	}
	messageReports := make([]*AgentRunReport, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		messageID := entry.Name()
		report, err := s.BuildReport(context.Background(), AgentReportQuery{
			TenantUUID: query.TenantUUID,
			SessionID:  query.SessionID,
			MessageID:  messageID,
			Source:     query.Source,
			Format:     query.Format,
		})
		if err != nil {
			continue
		}
		messageReports = append(messageReports, report)
	}
	sort.SliceStable(messageReports, func(i, j int) bool {
		return reportStartedAt(messageReports[i]).Before(reportStartedAt(messageReports[j]))
	})
	out := &AgentRunReport{
		ReportScope: "session",
		Format:      firstNonEmpty(query.Format, "json"),
		TenantUUID:  query.TenantUUID,
		SessionID:   query.SessionID,
		GeneratedBy: "powerx-agent-trace-local",
		GeneratedAt: time.Now().UTC(),
		Summary: map[string]any{
			"message_count": len(messageReports),
			"node_count":    0,
			"event_count":   0,
			"error_count":   0,
			"duration_ms":   int64(0),
		},
		Timeline:     []AgentTraceEvent{},
		Nodes:        []AgentTraceNodeSnapshot{},
		RunState:     &AgentRunStateSnapshot{Run: map[string]any{"session_id": query.SessionID, "tenant_uuid": query.TenantUUID}},
		Errors:       []map[string]any{},
		ArtifactRefs: []string{},
	}
	artifacts := map[string]struct{}{}
	for _, report := range messageReports {
		out.Timeline = append(out.Timeline, report.Timeline...)
		out.Nodes = append(out.Nodes, report.Nodes...)
		if report.RunState != nil && out.RunState != nil {
			out.RunState.Tasks = append(out.RunState.Tasks, report.RunState.Tasks...)
			out.RunState.PendingParams = append(out.RunState.PendingParams, report.RunState.PendingParams...)
			out.RunState.Results = append(out.RunState.Results, report.RunState.Results...)
			out.RunState.Errors = append(out.RunState.Errors, report.RunState.Errors...)
			out.RunState.TraceLinks = append(out.RunState.TraceLinks, report.RunState.TraceLinks...)
			if report.RunState.UpdatedAt.After(out.RunState.UpdatedAt) {
				out.RunState.UpdatedAt = report.RunState.UpdatedAt
			}
		}
		for _, item := range report.Errors {
			clone := map[string]any{}
			for k, v := range item {
				clone[k] = v
			}
			clone["message_id"] = report.MessageID
			out.Errors = append(out.Errors, clone)
		}
		for _, ref := range report.ArtifactRefs {
			if strings.TrimSpace(ref) != "" {
				artifacts[ref] = struct{}{}
			}
		}
		out.Summary["node_count"] = toInt(out.Summary["node_count"]) + len(report.Nodes)
		out.Summary["event_count"] = toInt(out.Summary["event_count"]) + len(report.Timeline)
		out.Summary["error_count"] = toInt(out.Summary["error_count"]) + len(report.Errors)
		out.Summary["duration_ms"] = toInt64(out.Summary["duration_ms"]) + toInt64(report.Summary["duration_ms"])
	}
	for ref := range artifacts {
		out.ArtifactRefs = append(out.ArtifactRefs, ref)
	}
	sort.Strings(out.ArtifactRefs)
	sort.SliceStable(out.Timeline, func(i, j int) bool {
		if out.Timeline[i].CreatedAt.Equal(out.Timeline[j].CreatedAt) {
			return out.Timeline[i].NodeSeq < out.Timeline[j].NodeSeq
		}
		return out.Timeline[i].CreatedAt.Before(out.Timeline[j].CreatedAt)
	})
	return out, nil
}

func (s *LocalSink) ListRuns(_ context.Context, query AgentRunListQuery) (AgentRunListResult, error) {
	tenantUUID := strings.TrimSpace(query.TenantUUID)
	if tenantUUID == "" {
		return AgentRunListResult{}, missingFieldsError("tenant_uuid")
	}
	limit := query.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := query.Offset
	if offset < 0 {
		offset = 0
	}
	status := strings.ToLower(strings.TrimSpace(query.Status))
	items, err := s.collectRunItems(tenantUUID, strings.TrimSpace(query.SessionID), status)
	if err != nil {
		return AgentRunListResult{}, err
	}
	total := len(items)
	if offset > total {
		items = []AgentRunListItem{}
	} else {
		end := offset + limit
		if end > total {
			end = total
		}
		items = items[offset:end]
	}
	return AgentRunListResult{Items: items, TenantUUID: tenantUUID, Total: total, Offset: offset, Limit: limit}, nil
}

func (s *LocalSink) collectRunItems(tenantUUID string, sessionFilter string, status string) ([]AgentRunListItem, error) {
	tenantDir := filepath.Join(s.rootDir, safePathPart(tenantUUID))
	sessionEntries, err := os.ReadDir(tenantDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []AgentRunListItem{}, nil
		}
		return nil, err
	}
	items := make([]AgentRunListItem, 0)
	for _, sessionEntry := range sessionEntries {
		if !sessionEntry.IsDir() {
			continue
		}
		sessionID := sessionEntry.Name()
		if sessionFilter != "" && sessionID != sessionFilter {
			continue
		}
		messageEntries, err := os.ReadDir(filepath.Join(tenantDir, sessionID))
		if err != nil {
			continue
		}
		for _, messageEntry := range messageEntries {
			if !messageEntry.IsDir() {
				continue
			}
			messageID := messageEntry.Name()
			run, err := readJSONFile[AgentRunTrace](filepath.Join(tenantDir, sessionID, messageID, "run.json"))
			if err != nil {
				continue
			}
			if status != "" && strings.ToLower(strings.TrimSpace(run.Status)) != status {
				continue
			}
			items = append(items, AgentRunListItem{
				TenantUUID: run.TenantUUID,
				SessionID:  run.SessionID,
				MessageID:  run.MessageID,
				RunID:      run.RunID,
				TraceID:    run.TraceID,
				AgentID:    run.AgentID,
				Status:     run.Status,
				NodeCount:  run.NodeCount,
				EventCount: run.EventCount,
				ErrorCount: run.ErrorCount,
				DurationMS: run.DurationMS,
				StartedAt:  run.StartedAt,
				EndedAt:    run.EndedAt,
				CreatedAt:  run.CreatedAt,
			})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return runListSortTime(items[i]).After(runListSortTime(items[j]))
	})
	return items, nil
}

func (s *LocalSink) ListSessions(ctx context.Context, query AgentRunListQuery) (AgentSessionListResult, error) {
	tenantUUID := strings.TrimSpace(query.TenantUUID)
	if tenantUUID == "" {
		return AgentSessionListResult{}, missingFieldsError("tenant_uuid")
	}
	limit := query.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := query.Offset
	if offset < 0 {
		offset = 0
	}
	allRuns, err := s.collectRunItems(tenantUUID, "", strings.ToLower(strings.TrimSpace(query.Status)))
	if err != nil {
		return AgentSessionListResult{}, err
	}
	bySession := map[string]*AgentSessionListItem{}
	for _, run := range allRuns {
		item := bySession[run.SessionID]
		if item == nil {
			item = &AgentSessionListItem{
				TenantUUID: run.TenantUUID,
				SessionID:  run.SessionID,
				AgentID:    run.AgentID,
				Status:     RunStatusCompleted,
				LatestAt:   runListSortTime(run),
			}
			bySession[run.SessionID] = item
		}
		item.MessageCount++
		item.NodeCount += run.NodeCount
		item.EventCount += run.EventCount
		item.ErrorCount += run.ErrorCount
		item.DurationMS += run.DurationMS
		if strings.TrimSpace(run.AgentID) != "" {
			item.AgentID = run.AgentID
		}
		if strings.ToLower(strings.TrimSpace(run.Status)) == RunStatusFailed || run.ErrorCount > 0 {
			item.Status = RunStatusFailed
		}
		if run.StartedAt != nil && (item.StartedAt == nil || run.StartedAt.Before(*item.StartedAt)) {
			t := *run.StartedAt
			item.StartedAt = &t
		}
		if run.EndedAt != nil && (item.EndedAt == nil || run.EndedAt.After(*item.EndedAt)) {
			t := *run.EndedAt
			item.EndedAt = &t
		}
		if latest := runListSortTime(run); latest.After(item.LatestAt) {
			item.LatestAt = latest
		}
	}
	items := make([]AgentSessionListItem, 0, len(bySession))
	for _, item := range bySession {
		items = append(items, *item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].LatestAt.After(items[j].LatestAt)
	})
	total := len(items)
	if offset > total {
		items = []AgentSessionListItem{}
	} else {
		end := offset + limit
		if end > total {
			end = total
		}
		items = items[offset:end]
	}
	return AgentSessionListResult{Items: items, TenantUUID: tenantUUID, Total: total, Offset: offset, Limit: limit}, nil
}

func (s *LocalSink) RenderMarkdown(report *AgentRunReport) string {
	if report == nil {
		return "# Agent Run Report\n\n报告为空。\n"
	}
	var b strings.Builder
	if report.ReportScope == "session" {
		fmt.Fprintf(&b, "# Agent Session Report\n\n")
	} else {
		fmt.Fprintf(&b, "# Agent Message Report\n\n")
	}
	fmt.Fprintf(&b, "- Tenant: `%s`\n", report.TenantUUID)
	fmt.Fprintf(&b, "- Session: `%s`\n", report.SessionID)
	if strings.TrimSpace(report.MessageID) != "" {
		fmt.Fprintf(&b, "- Message: `%s`\n", report.MessageID)
	}
	if strings.TrimSpace(report.RunID) != "" {
		fmt.Fprintf(&b, "- Run: `%s`\n", report.RunID)
	}
	if strings.TrimSpace(report.TraceID) != "" {
		fmt.Fprintf(&b, "- Trace: `%s`\n", report.TraceID)
	}
	fmt.Fprintf(&b, "- Generated At: `%s`\n\n", report.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "## Summary\n\n")
	for _, key := range []string{"status", "agent_id", "channel", "message_count", "node_count", "event_count", "error_count", "duration_ms"} {
		if v, ok := report.Summary[key]; ok {
			fmt.Fprintf(&b, "- %s: `%v`\n", key, v)
		}
	}
	if len(report.Nodes) > 0 {
		fmt.Fprintf(&b, "\n## Nodes\n\n")
		for _, node := range report.Nodes {
			fmt.Fprintf(&b, "### %03d %s `%s`\n\n", node.NodeSeq, node.NodeKind, firstNonEmpty(node.NodeRef, node.NodeID))
			fmt.Fprintf(&b, "- Status: `%s`\n", node.PhaseStatus)
			if node.SkillID != "" {
				fmt.Fprintf(&b, "- Skill: `%s`\n", node.SkillID)
			}
			if node.PluginID != "" {
				fmt.Fprintf(&b, "- Plugin: `%s`\n", node.PluginID)
			}
			if node.ErrorSummary != "" {
				fmt.Fprintf(&b, "- Error: `%s` %s\n", node.ErrorCode, node.ErrorSummary)
			}
			if len(node.InputSummary) > 0 {
				fmt.Fprintf(&b, "\nInput Summary:\n\n```json\n%s\n```\n", prettyJSON(node.InputSummary))
			}
			if len(node.OutputSummary) > 0 {
				fmt.Fprintf(&b, "\nOutput Summary:\n\n```json\n%s\n```\n", prettyJSON(node.OutputSummary))
			}
			fmt.Fprintf(&b, "\n")
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

func validateSessionReportQuery(query AgentReportQuery) error {
	return validateRequired(map[string]string{
		"tenant_uuid": query.TenantUUID,
		"session_id":  query.SessionID,
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

func reportStartedAt(report *AgentRunReport) time.Time {
	if report == nil || report.Summary == nil {
		return time.Time{}
	}
	switch v := report.Summary["started_at"].(type) {
	case time.Time:
		return v
	case string:
		t, _ := time.Parse(time.RFC3339Nano, v)
		return t
	default:
		return time.Time{}
	}
}

func prettyJSON(v any) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(data)
}

func toInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	default:
		return 0
	}
}

func toInt64(v any) int64 {
	switch x := v.(type) {
	case int:
		return int64(x)
	case int64:
		return x
	case float64:
		return int64(x)
	default:
		return 0
	}
}

func runListSortTime(item AgentRunListItem) time.Time {
	if item.EndedAt != nil && !item.EndedAt.IsZero() {
		return *item.EndedAt
	}
	if item.StartedAt != nil && !item.StartedAt.IsZero() {
		return *item.StartedAt
	}
	return item.CreatedAt
}
