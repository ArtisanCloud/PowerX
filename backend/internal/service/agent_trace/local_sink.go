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
	AppendRunStateEvent(ctx context.Context, meta AgentRunMeta, event string, payload any) error
	WriteNodeSnapshot(ctx context.Context, meta AgentRunMeta, snapshot AgentTraceNodeSnapshot) error
	CompleteRun(ctx context.Context, run AgentRunTrace) error
}

type ReportSource interface {
	BuildReport(ctx context.Context, query AgentReportQuery) (*AgentRunReport, error)
	ListRuns(ctx context.Context, query AgentRunListQuery) (AgentRunListResult, error)
	ListSessions(ctx context.Context, query AgentRunListQuery) (AgentSessionListResult, error)
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
	dir := s.runDir(run.TenantUUID, run.SessionID, run.MessageID, run.RunID)
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
	dir := s.runDir(event.TenantUUID, event.SessionID, event.MessageID, event.RunID)
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

func (s *LocalSink) AppendRunStateEvent(_ context.Context, meta AgentRunMeta, event string, payload any) error {
	if err := validateRunMeta(meta); err != nil {
		return err
	}
	event = strings.TrimSpace(event)
	if event == "" {
		return missingFieldsError("event")
	}
	dir := s.runDir(meta.TenantUUID, meta.SessionID, meta.MessageID, meta.RunID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "run_state.json")
	state, _ := readJSONFile[AgentRunStateSnapshot](path)
	state = reduceRunStateSnapshot(state, meta, event, payload)
	return writeJSONFile(path, state)
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
	dir := filepath.Join(s.runDir(meta.TenantUUID, meta.SessionID, meta.MessageID, meta.RunID), "nodes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := fmt.Sprintf("%03d_%s.json", snapshot.NodeSeq, safePathPart(snapshot.NodeKind))
	if snapshot.NodeSeq <= 0 {
		name = safePathPart(snapshot.NodeID) + ".json"
	}
	path := filepath.Join(dir, name)
	if existing, err := readJSONFile[AgentTraceNodeSnapshot](path); err == nil {
		snapshot = mergeNodeSnapshot(existing, snapshot)
	}
	return writeJSONFile(path, snapshot)
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
	dir := s.runDir(run.TenantUUID, run.SessionID, run.MessageID, run.RunID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	run.ArtifactRoot = filepath.Join(dir, "artifacts")
	if err := writeJSONFile(filepath.Join(dir, "run.json"), run); err != nil {
		return err
	}
	return s.writeRunReport(dir, run)
}

func (s *LocalSink) writeRunReport(dir string, run AgentRunTrace) error {
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

func (s *LocalSink) runDir(tenantUUID, sessionID, messageID, runID string) string {
	return filepath.Join(s.rootDir, safePathPart(tenantUUID), safePathPart(sessionID), safePathPart(messageID), safePathPart(runID))
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

func mergeNodeSnapshot(prev, next AgentTraceNodeSnapshot) AgentTraceNodeSnapshot {
	out := prev
	if strings.TrimSpace(next.NodeID) != "" {
		out.NodeID = next.NodeID
	}
	if next.NodeSeq != 0 {
		out.NodeSeq = next.NodeSeq
	}
	if strings.TrimSpace(next.NodeKind) != "" {
		out.NodeKind = next.NodeKind
	}
	if strings.TrimSpace(next.NodeRef) != "" {
		out.NodeRef = next.NodeRef
	}
	if strings.TrimSpace(next.PhaseStatus) != "" {
		out.PhaseStatus = next.PhaseStatus
	}
	if next.InputSummary != nil {
		out.InputSummary = next.InputSummary
	}
	if next.OutputSummary != nil {
		out.OutputSummary = next.OutputSummary
	}
	if strings.TrimSpace(next.ContextRef) != "" {
		out.ContextRef = next.ContextRef
	}
	if strings.TrimSpace(next.SkillID) != "" {
		out.SkillID = next.SkillID
	}
	if strings.TrimSpace(next.PluginID) != "" {
		out.PluginID = next.PluginID
	}
	if strings.TrimSpace(next.CapabilityID) != "" {
		out.CapabilityID = next.CapabilityID
	}
	if strings.TrimSpace(next.ExecutorPath) != "" {
		out.ExecutorPath = next.ExecutorPath
	}
	if next.PromptTokens != nil {
		out.PromptTokens = next.PromptTokens
	}
	if next.CompletionTokens != nil {
		out.CompletionTokens = next.CompletionTokens
	}
	if next.CachedTokens != nil {
		out.CachedTokens = next.CachedTokens
	}
	if next.TrimActions != nil {
		out.TrimActions = next.TrimActions
	}
	if next.Attributes != nil {
		if out.Attributes == nil {
			out.Attributes = map[string]any{}
		}
		for k, v := range next.Attributes {
			out.Attributes[k] = v
		}
	}
	if strings.TrimSpace(next.ErrorCode) != "" {
		out.ErrorCode = next.ErrorCode
	}
	if strings.TrimSpace(next.ErrorSummary) != "" {
		out.ErrorSummary = next.ErrorSummary
	}
	if next.ArtifactRefs != nil {
		out.ArtifactRefs = next.ArtifactRefs
	}
	if next.StartedAt != nil {
		out.StartedAt = next.StartedAt
	}
	if next.EndedAt != nil {
		out.EndedAt = next.EndedAt
	}
	return out
}

func reduceRunStateSnapshot(state AgentRunStateSnapshot, meta AgentRunMeta, event string, payload any) AgentRunStateSnapshot {
	now := time.Now().UTC()
	if state.Run == nil {
		state.Run = map[string]any{}
	}
	mergeRunStateIdentity(state.Run, meta)
	mergeRunStatePayloadIdentity(state.Run, payload)
	state.UpdatedAt = now
	switch event {
	case "agent_run.started":
		mergeRunStatePayloadIdentity(state.Run, payload)
	case "agent_run.response_plan":
		state.ResponsePlan = normalizeRunStateMapPayload(payload)
	case "agent_run.intent_detected":
		state.Intent = normalizeRunStateMapPayload(payload)
	case "agent_run.plan_created":
		state.Plan = normalizeRunStateMapPayload(payload)
		state.Summary = normalizeRunStateMapPayload(state.Plan["summary"])
		for _, task := range runStateTasksFromPlanPayload(state.Plan["tasks"]) {
			if task.RunID == "" {
				task.RunID = meta.RunID
			}
			if task.SessionID == "" {
				task.SessionID = meta.SessionID
			}
			if task.MessageID == "" {
				task.MessageID = meta.MessageID
			}
			if task.TraceID == "" {
				task.TraceID = meta.TraceID
			}
			state.Tasks = upsertRunStateTask(state.Tasks, task)
		}
	case "agent_run.task_status", "agent_run.task_started", "agent_run.awaiting_params", "agent_run.task_completed", "agent_run.task_failed":
		task := runStateTaskFromPayload(payload)
		if task.RunID == "" {
			task.RunID = meta.RunID
		}
		if task.SessionID == "" {
			task.SessionID = meta.SessionID
		}
		if task.MessageID == "" {
			task.MessageID = meta.MessageID
		}
		if task.TraceID == "" {
			task.TraceID = meta.TraceID
		}
		if task.AgentID == "" {
			task.AgentID = meta.AgentID
		}
		switch event {
		case "agent_run.task_started":
			if task.Status == "" {
				task.Status = "running"
			}
		case "agent_run.awaiting_params":
			task.Status = "awaiting_params"
		case "agent_run.task_completed":
			if task.Status == "" {
				task.Status = "completed"
			}
		case "agent_run.task_failed":
			if task.Status == "" {
				task.Status = "failed"
			}
		}
		if task.TaskID == "" {
			task.TaskID = firstNonEmpty(task.NodeRef, task.SkillID, task.CapabilityID, event)
		}
		state.Tasks = upsertRunStateTask(state.Tasks, task)
		state.PendingParams = filterRunStateTasks(state.Tasks, "awaiting_params")
		state.Results = filterRunStateTasks(state.Tasks, "completed")
		state.Errors = filterRunStateTasks(state.Tasks, "failed")
		state.Summary = buildRunStateSummary(meta, state.Tasks)
	case "agent_run.final":
		state.Final = normalizeRunStateMapPayload(payload)
	case "agent_run.ended":
		state.Ended = true
		mergeRunStatePayloadIdentity(state.Run, payload)
	}
	state.TraceLinks = buildRunStateTraceLinks(meta, state.Tasks)
	return state
}

func mergeRunStateIdentity(run map[string]any, meta AgentRunMeta) {
	if run == nil {
		return
	}
	for key, value := range map[string]string{
		"run_id":      meta.RunID,
		"tenant_uuid": meta.TenantUUID,
		"agent_id":    meta.AgentID,
		"session_id":  meta.SessionID,
		"message_id":  meta.MessageID,
		"trace_id":    meta.TraceID,
		"plan_id":     meta.PlanID,
	} {
		if strings.TrimSpace(value) != "" {
			run[key] = value
		}
	}
}

func mergeRunStatePayloadIdentity(run map[string]any, payload any) {
	if run == nil {
		return
	}
	m := normalizeRunStateMapPayload(payload)
	for _, key := range []string{"run_id", "tenant_uuid", "agent_id", "session_id", "message_id", "trace_id", "plan_id"} {
		if v, ok := m[key]; ok && strings.TrimSpace(fmt.Sprint(v)) != "" {
			run[key] = v
		}
	}
}

func normalizeRunStateMapPayload(payload any) map[string]any {
	if payload == nil {
		return nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return map[string]any{"value": fmt.Sprint(payload)}
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{"value": string(raw)}
	}
	if nested, ok := out["payload"].(map[string]any); ok {
		for _, key := range []string{"run_id", "session_id", "message_id", "trace_id"} {
			if _, exists := nested[key]; !exists {
				if v, ok := out[key]; ok {
					nested[key] = v
				}
			}
		}
		return nested
	}
	return out
}

func runStateTaskFromPayload(payload any) AgentTaskStateItem {
	m := normalizeRunStateMapPayload(payload)
	task := AgentTaskStateItem{
		RunID:         stringFromRunState(m["run_id"]),
		SessionID:     stringFromRunState(m["session_id"]),
		MessageID:     stringFromRunState(m["message_id"]),
		TraceID:       stringFromRunState(m["trace_id"]),
		TaskID:        firstNonEmpty(stringFromRunState(m["task_id"]), stringFromRunState(m["node_id"])),
		ParentTaskID:  stringFromRunState(m["parent_task_id"]),
		DependsOn:     stringListFromRunState(m["depends_on"]),
		Stage:         intFromRunState(m["stage"]),
		ParallelGroup: stringFromRunState(m["parallel_group"]),
		TeamID:        stringFromRunState(m["team_id"]),
		AgentID:       stringFromRunState(m["agent_id"]),
		AgentKey:      stringFromRunState(m["agent_key"]),
		AgentName:     stringFromRunState(m["agent_name"]),
		NodeKind:      stringFromRunState(m["node_kind"]),
		NodeRef:       stringFromRunState(m["node_ref"]),
		SkillID:       stringFromRunState(m["skill_id"]),
		CapabilityID:  stringFromRunState(m["capability_id"]),
		Action:        stringFromRunState(m["action"]),
		FailurePolicy: stringFromRunState(m["failure_policy"]),
		Status:        stringFromRunState(m["status"]),
		Message:       firstNonEmpty(stringFromRunState(m["message"]), stringFromRunState(m["display_message"])),
		Summary:       firstNonEmpty(stringFromRunState(m["summary"]), stringFromRunState(m["result_message"])),
		Result:        firstRunStateValue(m["result"], m["result_summary"], m["data"]),
		Error:         runStateTaskError(m),
		UpdatedAt:     firstNonEmpty(stringFromRunState(m["updated_at"]), time.Now().UTC().Format(time.RFC3339Nano)),
	}
	task.MissingFields = stringListFromRunState(m["missing_fields"])
	if params, ok := m["collected_params"].(map[string]any); ok {
		task.CollectedParams = params
	}
	task.Links = mapListFromRunState(m["links"])
	return task
}

func runStateTaskError(m map[string]any) any {
	status := strings.ToLower(strings.TrimSpace(stringFromRunState(m["status"])))
	switch status {
	case "failed", "canceled", "cancelled":
		return firstRunStateValue(m["error"], m["detail"], m["message"])
	default:
		return firstRunStateValue(m["error"], m["detail"])
	}
}

func upsertRunStateTask(tasks []AgentTaskStateItem, task AgentTaskStateItem) []AgentTaskStateItem {
	key := strings.TrimSpace(task.TaskID)
	for i := range tasks {
		if strings.TrimSpace(tasks[i].TaskID) == key && key != "" {
			tasks[i] = mergeRunStateTask(tasks[i], task)
			return tasks
		}
	}
	return append(tasks, task)
}

func mergeRunStateTask(prev, next AgentTaskStateItem) AgentTaskStateItem {
	raw, _ := json.Marshal(prev)
	out := AgentTaskStateItem{}
	_ = json.Unmarshal(raw, &out)
	overlay, _ := json.Marshal(next)
	var m map[string]any
	_ = json.Unmarshal(overlay, &m)
	base, _ := json.Marshal(out)
	var bm map[string]any
	_ = json.Unmarshal(base, &bm)
	for k, v := range m {
		if v == nil {
			continue
		}
		if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
			continue
		}
		if arr, ok := v.([]any); ok && len(arr) == 0 {
			continue
		}
		bm[k] = v
	}
	merged, _ := json.Marshal(bm)
	_ = json.Unmarshal(merged, &out)
	return out
}

func filterRunStateTasks(tasks []AgentTaskStateItem, status string) []AgentTaskStateItem {
	out := make([]AgentTaskStateItem, 0)
	for _, task := range tasks {
		if strings.EqualFold(strings.TrimSpace(task.Status), status) {
			out = append(out, task)
		}
	}
	return out
}

func runStateTasksFromPlanPayload(raw any) []AgentTaskStateItem {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tasks := make([]AgentTaskStateItem, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		stage := intFromRunState(m["stage"])
		parallelGroup := stringFromRunState(m["parallel_group"])
		if parallelGroup == "" && stage > 0 {
			parallelGroup = fmt.Sprintf("stage_%d", stage)
		}
		tasks = append(tasks, AgentTaskStateItem{
			TaskID:        firstNonEmpty(stringFromRunState(m["task_id"]), stringFromRunState(m["node_id"])),
			ParentTaskID:  stringFromRunState(m["parent_task_id"]),
			DependsOn:     stringListFromRunState(m["depends_on"]),
			Stage:         stage,
			ParallelGroup: parallelGroup,
			TeamID:        stringFromRunState(m["team_id"]),
			AgentID:       stringFromRunState(m["agent_id"]),
			AgentKey:      stringFromRunState(m["agent_key"]),
			AgentName:     stringFromRunState(m["agent_name"]),
			NodeKind:      stringFromRunState(m["node_kind"]),
			NodeRef:       stringFromRunState(m["node_ref"]),
			SkillID:       stringFromRunState(m["skill_id"]),
			CapabilityID:  stringFromRunState(m["capability_id"]),
			Action:        stringFromRunState(m["action"]),
			FailurePolicy: stringFromRunState(m["failure_policy"]),
			Status:        firstNonEmpty(stringFromRunState(m["status"]), "pending"),
			UpdatedAt:     now,
		})
	}
	return tasks
}

func buildRunStateSummary(meta AgentRunMeta, tasks []AgentTaskStateItem) map[string]any {
	summary := map[string]any{
		"run_id":      meta.RunID,
		"session_id":  meta.SessionID,
		"message_id":  meta.MessageID,
		"trace_id":    meta.TraceID,
		"status":      "idle",
		"total_tasks": len(tasks),
		"updated_at":  time.Now().UTC().Format(time.RFC3339Nano),
	}
	maxStage := 0
	counts := map[string]int{}
	for _, task := range tasks {
		status := strings.TrimSpace(task.Status)
		if status == "" {
			status = "pending"
		}
		counts[status]++
		if task.Stage > maxStage {
			maxStage = task.Stage
		}
	}
	summary["pending_tasks"] = counts["pending"]
	summary["awaiting_tasks"] = counts["awaiting_params"]
	summary["running_tasks"] = counts["running"]
	summary["completed_tasks"] = counts["completed"]
	summary["failed_tasks"] = counts["failed"]
	summary["skipped_tasks"] = counts["skipped"]
	summary["total_stages"] = maxStage
	switch {
	case counts["failed"] > 0:
		summary["status"] = "failed"
	case counts["awaiting_params"] > 0:
		summary["status"] = "awaiting_params"
		summary["blocked_reason"] = "missing_required_params"
	case counts["running"] > 0:
		summary["status"] = "running"
	case len(tasks) > 0 && counts["completed"]+counts["skipped"] == len(tasks):
		summary["status"] = "completed"
	case len(tasks) > 0:
		summary["status"] = "pending"
	}
	return summary
}

func buildRunStateTraceLinks(meta AgentRunMeta, tasks []AgentTaskStateItem) []map[string]any {
	links := []map[string]any{
		{
			"scope":       "message",
			"tenant_uuid": meta.TenantUUID,
			"session_id":  meta.SessionID,
			"message_id":  meta.MessageID,
			"run_id":      meta.RunID,
			"trace_id":    meta.TraceID,
		},
	}
	for _, task := range tasks {
		if strings.TrimSpace(task.TaskID) == "" {
			continue
		}
		links = append(links, map[string]any{
			"scope":       "task",
			"tenant_uuid": meta.TenantUUID,
			"session_id":  meta.SessionID,
			"message_id":  meta.MessageID,
			"run_id":      meta.RunID,
			"trace_id":    meta.TraceID,
			"task_id":     task.TaskID,
			"node_id":     firstNonEmpty(task.TaskID, task.NodeRef),
		})
	}
	return links
}

func stringFromRunState(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func stringListFromRunState(v any) []string {
	switch x := v.(type) {
	case []string:
		return append([]string(nil), x...)
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s := strings.TrimSpace(fmt.Sprint(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func intFromRunState(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case float32:
		return int(x)
	default:
		var out int
		_, _ = fmt.Sscanf(stringFromRunState(v), "%d", &out)
		return out
	}
}

func mapListFromRunState(v any) []map[string]any {
	switch x := v.(type) {
	case []map[string]any:
		return append([]map[string]any(nil), x...)
	case []any:
		out := make([]map[string]any, 0, len(x))
		for _, item := range x {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func firstRunStateValue(values ...any) any {
	for _, value := range values {
		if value == nil {
			continue
		}
		if s, ok := value.(string); ok && strings.TrimSpace(s) == "" {
			continue
		}
		return value
	}
	return nil
}

func defaultTime(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}
