package agent_trace

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type AgentTraceLogger interface {
	StartRun(ctx context.Context, meta AgentRunMeta) (*AgentRunContext, error)
	AppendEvent(ctx context.Context, event AgentTraceEvent) error
	AppendRunStateEvent(ctx context.Context, meta AgentRunMeta, event string, payload any) error
	StartNode(ctx context.Context, node AgentTraceNode) error
	EndNode(ctx context.Context, result AgentTraceNodeResult) error
	FailNode(ctx context.Context, failure AgentTraceNodeFailure) error
	CompleteRun(ctx context.Context, result AgentRunResult) error
	BuildReport(ctx context.Context, query AgentReportQuery) (*AgentRunReport, error)
}

type Logger struct {
	cfg   Config
	sinks []Sink
	mu    sync.Mutex
	runs  map[string]*runCounters
}

type runCounters struct {
	startedAt    time.Time
	nodeIDs      map[string]struct{}
	eventCount   int
	errorCount   int
	warningCount int
}

func NewLogger(cfg Config, sinks ...Sink) *Logger {
	cfg = cfg.normalized()
	if len(sinks) == 0 && cfg.Enabled {
		if cfg.LocalEnabled {
			sinks = append(sinks, NewLocalSink(cfg.LocalDir))
		}
		if cfg.LokiEnabled {
			sinks = append(sinks, NewLokiSink(cfg.LokiEndpoint, cfg.LokiLabels))
		}
	}
	return &Logger{
		cfg:   cfg,
		sinks: sinks,
		runs:  map[string]*runCounters{},
	}
}

func NewLoggerFromEnv() *Logger {
	return NewLogger(ConfigFromEnv())
}

func (l *Logger) StartRun(ctx context.Context, meta AgentRunMeta) (*AgentRunContext, error) {
	if l == nil || !l.cfg.Enabled {
		return nil, nil
	}
	if err := validateRunMeta(meta); err != nil {
		return nil, err
	}
	if len(l.sinks) == 0 {
		return nil, &TraceError{Code: ErrCodeSinkUnavailable, Message: "at least one agent trace sink is required"}
	}
	now := time.Now().UTC()
	run := AgentRunTrace{
		TraceID:             strings.TrimSpace(meta.TraceID),
		RunID:               strings.TrimSpace(meta.RunID),
		TenantUUID:          strings.TrimSpace(meta.TenantUUID),
		UserUUID:            strings.TrimSpace(meta.UserUUID),
		AgentID:             strings.TrimSpace(meta.AgentID),
		SessionID:           strings.TrimSpace(meta.SessionID),
		MessageID:           strings.TrimSpace(meta.MessageID),
		PlanID:              strings.TrimSpace(meta.PlanID),
		Channel:             strings.TrimSpace(meta.Channel),
		Status:              RunStatusRunning,
		UserMessageDigest:   strings.TrimSpace(meta.UserMessageDigest),
		FinalResponseDigest: strings.TrimSpace(meta.FinalResponseDigest),
		StartedAt:           &now,
		CreatedAt:           now,
	}
	if err := l.eachSink(ctx, func(s Sink) error { return s.StartRun(ctx, run) }); err != nil {
		return nil, err
	}
	l.mu.Lock()
	l.runs[meta.RunID] = &runCounters{startedAt: now, nodeIDs: map[string]struct{}{}}
	l.mu.Unlock()
	return &AgentRunContext{Meta: meta, Trace: run, StartedAt: now}, nil
}

func (l *Logger) AppendEvent(ctx context.Context, event AgentTraceEvent) error {
	if l == nil || !l.cfg.Enabled {
		return nil
	}
	event = normalizeEvent(event)
	if err := validateTraceEvent(event); err != nil {
		return err
	}
	if err := l.eachSink(ctx, func(s Sink) error { return s.AppendEvent(ctx, event) }); err != nil {
		return err
	}
	l.recordEvent(event)
	return nil
}

func (l *Logger) AppendRunStateEvent(ctx context.Context, meta AgentRunMeta, event string, payload any) error {
	if l == nil || !l.cfg.Enabled {
		return nil
	}
	if err := validateRunMeta(meta); err != nil {
		return err
	}
	event = strings.TrimSpace(event)
	if event == "" {
		return missingFieldsError("event")
	}
	return l.eachSink(ctx, func(s Sink) error { return s.AppendRunStateEvent(ctx, meta, event, payload) })
}

func (l *Logger) StartNode(ctx context.Context, node AgentTraceNode) error {
	if l == nil || !l.cfg.Enabled {
		return nil
	}
	if err := validateRunMeta(node.AgentRunMeta); err != nil {
		return err
	}
	if strings.TrimSpace(node.NodeID) == "" {
		return missingFieldsError("node_id")
	}
	if strings.TrimSpace(node.NodeKind) == "" {
		return missingFieldsError("node_kind")
	}
	startedAt := node.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	event := AgentTraceEvent{
		TraceID:      node.TraceID,
		RunID:        node.RunID,
		TenantUUID:   node.TenantUUID,
		UserUUID:     node.UserUUID,
		AgentID:      node.AgentID,
		SessionID:    node.SessionID,
		MessageID:    node.MessageID,
		PlanID:       node.PlanID,
		NodeID:       node.NodeID,
		NodeSeq:      node.NodeSeq,
		NodeKind:     node.NodeKind,
		NodeRef:      node.NodeRef,
		Phase:        EventPhaseStart,
		Status:       EventStatusRunning,
		InputDigest:  node.InputDigest,
		ArtifactRefs: append([]string(nil), node.ArtifactRefs...),
		Attributes:   node.Attributes,
		CreatedAt:    startedAt,
	}
	if err := l.AppendEvent(ctx, event); err != nil {
		return err
	}
	snapshot := AgentTraceNodeSnapshot{
		NodeID:       node.NodeID,
		NodeSeq:      node.NodeSeq,
		NodeKind:     node.NodeKind,
		NodeRef:      node.NodeRef,
		PhaseStatus:  EventStatusRunning,
		InputSummary: node.InputSummary,
		ContextRef:   node.ContextRef,
		SkillID:      node.SkillID,
		PluginID:     node.PluginID,
		CapabilityID: node.CapabilityID,
		ExecutorPath: node.ExecutorPath,
		ArtifactRefs: append([]string(nil), node.ArtifactRefs...),
		Attributes:   cloneMap(node.Attributes),
		StartedAt:    &startedAt,
	}
	return l.eachSink(ctx, func(s Sink) error { return s.WriteNodeSnapshot(ctx, node.AgentRunMeta, snapshot) })
}

func (l *Logger) EndNode(ctx context.Context, result AgentTraceNodeResult) error {
	return l.finishNode(ctx, result, "", "")
}

func (l *Logger) FailNode(ctx context.Context, failure AgentTraceNodeFailure) error {
	return l.finishNode(ctx, failure.AgentTraceNodeResult, failure.ErrorCode, failure.ErrorSummary)
}

func (l *Logger) CompleteRun(ctx context.Context, result AgentRunResult) error {
	if l == nil || !l.cfg.Enabled {
		return nil
	}
	if err := validateRunMeta(result.AgentRunMeta); err != nil {
		return err
	}
	now := result.EndedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	status := strings.TrimSpace(result.Status)
	if status == "" {
		status = RunStatusCompleted
	}
	counters := l.snapshotCounters(result.RunID)
	startedAt := result.StartedAt
	if startedAt.IsZero() {
		startedAt = counters.startedAt
	}
	var startedPtr *time.Time
	if !startedAt.IsZero() {
		startedPtr = &startedAt
	}
	durationMS := result.DurationMS
	if durationMS <= 0 && startedPtr != nil {
		durationMS = now.Sub(*startedPtr).Milliseconds()
	}
	run := AgentRunTrace{
		TraceID:             result.TraceID,
		RunID:               result.RunID,
		TenantUUID:          result.TenantUUID,
		UserUUID:            result.UserUUID,
		AgentID:             result.AgentID,
		SessionID:           result.SessionID,
		MessageID:           result.MessageID,
		PlanID:              result.PlanID,
		Channel:             result.Channel,
		Status:              status,
		NodeCount:           maxInt(result.NodeCount, len(counters.nodeIDs)),
		EventCount:          maxInt(result.EventCount, counters.eventCount),
		ErrorCount:          maxInt(result.ErrorCount, counters.errorCount),
		WarningCount:        maxInt(result.WarningCount, counters.warningCount),
		DurationMS:          durationMS,
		UserMessageDigest:   result.UserMessageDigest,
		FinalResponseDigest: firstNonEmpty(result.FinalResponseDigest, result.AgentRunMeta.FinalResponseDigest),
		StartedAt:           startedPtr,
		EndedAt:             &now,
		CreatedAt:           now,
	}
	if err := l.eachSink(ctx, func(s Sink) error { return s.CompleteRun(ctx, run) }); err != nil {
		return err
	}
	l.mu.Lock()
	delete(l.runs, result.RunID)
	l.mu.Unlock()
	return nil
}

func (l *Logger) finishNode(ctx context.Context, result AgentTraceNodeResult, errorCode, errorSummary string) error {
	if l == nil || !l.cfg.Enabled {
		return nil
	}
	if err := validateRunMeta(result.AgentRunMeta); err != nil {
		return err
	}
	if strings.TrimSpace(result.NodeID) == "" {
		return missingFieldsError("node_id")
	}
	if strings.TrimSpace(result.NodeKind) == "" {
		return missingFieldsError("node_kind")
	}
	endedAt := result.EndedAt
	if endedAt.IsZero() {
		endedAt = time.Now().UTC()
	}
	phase := EventPhaseEnd
	status := EventStatusSuccess
	if strings.TrimSpace(errorCode) != "" || strings.TrimSpace(errorSummary) != "" {
		phase = EventPhaseError
		status = EventStatusError
	}
	durationMS := int64(0)
	if !result.StartedAt.IsZero() {
		durationMS = endedAt.Sub(result.StartedAt).Milliseconds()
	}
	event := AgentTraceEvent{
		TraceID:      result.TraceID,
		RunID:        result.RunID,
		TenantUUID:   result.TenantUUID,
		UserUUID:     result.UserUUID,
		AgentID:      result.AgentID,
		SessionID:    result.SessionID,
		MessageID:    result.MessageID,
		PlanID:       result.PlanID,
		NodeID:       result.NodeID,
		NodeSeq:      result.NodeSeq,
		NodeKind:     result.NodeKind,
		NodeRef:      result.NodeRef,
		Phase:        phase,
		Status:       status,
		DurationMS:   durationMS,
		OutputDigest: result.OutputDigest,
		ArtifactRefs: append([]string(nil), result.ArtifactRefs...),
		ErrorCode:    strings.TrimSpace(errorCode),
		ErrorSummary: strings.TrimSpace(errorSummary),
		Attributes:   result.Attributes,
		CreatedAt:    endedAt,
	}
	if err := l.AppendEvent(ctx, event); err != nil {
		return err
	}
	var startedPtr *time.Time
	if !result.StartedAt.IsZero() {
		startedPtr = &result.StartedAt
	}
	snapshot := AgentTraceNodeSnapshot{
		NodeID:           result.NodeID,
		NodeSeq:          result.NodeSeq,
		NodeKind:         result.NodeKind,
		NodeRef:          result.NodeRef,
		PhaseStatus:      status,
		OutputSummary:    result.OutputSummary,
		PromptTokens:     result.PromptTokens,
		CompletionTokens: result.CompletionTokens,
		CachedTokens:     result.CachedTokens,
		TrimActions:      result.TrimActions,
		ErrorCode:        strings.TrimSpace(errorCode),
		ErrorSummary:     strings.TrimSpace(errorSummary),
		ArtifactRefs:     append([]string(nil), result.ArtifactRefs...),
		Attributes:       cloneMap(result.Attributes),
		StartedAt:        startedPtr,
		EndedAt:          &endedAt,
	}
	return l.eachSink(ctx, func(s Sink) error { return s.WriteNodeSnapshot(ctx, result.AgentRunMeta, snapshot) })
}

func (l *Logger) eachSink(ctx context.Context, fn func(Sink) error) error {
	if len(l.sinks) == 0 {
		return &TraceError{Code: ErrCodeSinkUnavailable, Message: "at least one agent trace sink is required"}
	}
	var errs []error
	for _, sink := range l.sinks {
		if sink == nil {
			continue
		}
		if err := fn(sink); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (l *Logger) recordEvent(event AgentTraceEvent) {
	l.mu.Lock()
	defer l.mu.Unlock()
	c := l.runs[event.RunID]
	if c == nil {
		c = &runCounters{nodeIDs: map[string]struct{}{}}
		l.runs[event.RunID] = c
	}
	if event.NodeID != "" {
		c.nodeIDs[event.NodeID] = struct{}{}
	}
	c.eventCount++
	if event.Status == EventStatusError || event.Phase == EventPhaseError {
		c.errorCount++
	}
	if strings.EqualFold(fmt.Sprint(event.Attributes["severity"]), "warning") {
		c.warningCount++
	}
}

func (l *Logger) snapshotCounters(runID string) runCounters {
	l.mu.Lock()
	defer l.mu.Unlock()
	c := l.runs[runID]
	if c == nil {
		return runCounters{nodeIDs: map[string]struct{}{}}
	}
	clone := runCounters{
		startedAt:    c.startedAt,
		nodeIDs:      map[string]struct{}{},
		eventCount:   c.eventCount,
		errorCount:   c.errorCount,
		warningCount: c.warningCount,
	}
	for k, v := range c.nodeIDs {
		clone.nodeIDs[k] = v
	}
	return clone
}

func normalizeEvent(event AgentTraceEvent) AgentTraceEvent {
	event.TraceID = strings.TrimSpace(event.TraceID)
	event.RunID = strings.TrimSpace(event.RunID)
	event.TenantUUID = strings.TrimSpace(event.TenantUUID)
	event.AgentID = strings.TrimSpace(event.AgentID)
	event.SessionID = strings.TrimSpace(event.SessionID)
	event.MessageID = strings.TrimSpace(event.MessageID)
	event.NodeID = strings.TrimSpace(event.NodeID)
	event.NodeKind = strings.TrimSpace(event.NodeKind)
	event.Phase = strings.TrimSpace(event.Phase)
	event.Status = strings.TrimSpace(event.Status)
	if event.Phase == "" {
		event.Phase = EventPhaseDelta
	}
	if event.Status == "" {
		event.Status = EventStatusRunning
	}
	event.CreatedAt = defaultTime(event.CreatedAt)
	return event
}

func validateRunTrace(run AgentRunTrace) error {
	return validateRequired(map[string]string{
		"trace_id":    run.TraceID,
		"run_id":      run.RunID,
		"tenant_uuid": run.TenantUUID,
		"agent_id":    run.AgentID,
		"session_id":  run.SessionID,
		"message_id":  run.MessageID,
	})
}

func validateRunMeta(meta AgentRunMeta) error {
	return validateRequired(map[string]string{
		"trace_id":    meta.TraceID,
		"run_id":      meta.RunID,
		"tenant_uuid": meta.TenantUUID,
		"agent_id":    meta.AgentID,
		"session_id":  meta.SessionID,
		"message_id":  meta.MessageID,
	})
}

func validateTraceEvent(event AgentTraceEvent) error {
	return validateRequired(map[string]string{
		"trace_id":    event.TraceID,
		"run_id":      event.RunID,
		"tenant_uuid": event.TenantUUID,
		"agent_id":    event.AgentID,
		"session_id":  event.SessionID,
		"message_id":  event.MessageID,
		"node_id":     event.NodeID,
		"node_kind":   event.NodeKind,
		"phase":       event.Phase,
		"status":      event.Status,
	})
}

func validateRequired(fields map[string]string) error {
	missing := make([]string, 0, len(fields))
	for k, v := range fields {
		if strings.TrimSpace(v) == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return missingFieldsError(missing...)
	}
	return nil
}

func missingFieldsError(fields ...string) error {
	return &TraceError{
		Code:          ErrCodeContextMissing,
		Message:       "agent trace context missing required fields",
		MissingFields: fields,
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
