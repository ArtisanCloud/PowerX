package agent_trace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type LokiSink struct {
	pushURL string
	labels  map[string]string
	client  *http.Client
}

func NewLokiSink(endpoint string, labels map[string]string) *LokiSink {
	return &LokiSink{
		pushURL: normalizeLokiPushURL(endpoint),
		labels:  mergeLabels(map[string]string{"service": "powerx-agent", "component": "agent-runtime"}, labels),
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *LokiSink) StartRun(ctx context.Context, run AgentRunTrace) error {
	if err := validateRunTrace(run); err != nil {
		return err
	}
	return s.push(ctx, runLabels(s.labels, run, "", run.Status), map[string]any{
		"type": "run_start",
		"run":  run,
	}, run.CreatedAt)
}

func (s *LokiSink) AppendEvent(ctx context.Context, event AgentTraceEvent) error {
	if err := validateTraceEvent(event); err != nil {
		return err
	}
	return s.push(ctx, eventLabels(s.labels, event), event, event.CreatedAt)
}

func (s *LokiSink) WriteNodeSnapshot(ctx context.Context, meta AgentRunMeta, snapshot AgentTraceNodeSnapshot) error {
	if err := validateRunMeta(meta); err != nil {
		return err
	}
	labels := runLabels(s.labels, AgentRunTrace{
		TraceID:    meta.TraceID,
		RunID:      meta.RunID,
		TenantUUID: meta.TenantUUID,
		AgentID:    meta.AgentID,
		SessionID:  meta.SessionID,
		MessageID:  meta.MessageID,
	}, snapshot.NodeKind, snapshot.PhaseStatus)
	return s.push(ctx, labels, map[string]any{
		"type":     "node_snapshot",
		"run_id":   meta.RunID,
		"node_id":  snapshot.NodeID,
		"snapshot": snapshot,
	}, time.Now().UTC())
}

func (s *LokiSink) CompleteRun(ctx context.Context, run AgentRunTrace) error {
	if err := validateRunTrace(run); err != nil {
		return err
	}
	ts := run.CreatedAt
	if run.EndedAt != nil {
		ts = *run.EndedAt
	}
	return s.push(ctx, runLabels(s.labels, run, "", run.Status), map[string]any{
		"type": "run_complete",
		"run":  run,
	}, ts)
}

func (s *LokiSink) push(ctx context.Context, labels map[string]string, line any, ts time.Time) error {
	if strings.TrimSpace(s.pushURL) == "" {
		return &TraceError{Code: ErrCodeSinkUnavailable, Message: "loki endpoint is required"}
	}
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	payloadLine, err := json.Marshal(line)
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]any{
		"streams": []map[string]any{
			{
				"stream": labels,
				"values": [][]string{{fmt.Sprintf("%d", ts.UnixNano()), string(payloadLine)}},
			},
		},
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.pushURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("loki push failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

func runLabels(base map[string]string, run AgentRunTrace, nodeKind, status string) map[string]string {
	labels := mergeLabels(base, map[string]string{
		"tenant_uuid": run.TenantUUID,
		"agent_id":    run.AgentID,
		"session_id":  run.SessionID,
		"message_id":  run.MessageID,
		"run_id":      run.RunID,
	})
	if nodeKind != "" {
		labels["node_kind"] = sanitizeLabelValue(nodeKind)
	}
	if status != "" {
		labels["status"] = sanitizeLabelValue(status)
	}
	return labels
}

func eventLabels(base map[string]string, event AgentTraceEvent) map[string]string {
	labels := mergeLabels(base, map[string]string{
		"tenant_uuid": event.TenantUUID,
		"agent_id":    event.AgentID,
		"session_id":  event.SessionID,
		"message_id":  event.MessageID,
		"run_id":      event.RunID,
		"node_kind":   event.NodeKind,
		"status":      event.Status,
	})
	return labels
}

func normalizeLokiPushURL(raw string) string {
	base := strings.TrimSpace(raw)
	if base == "" {
		return ""
	}
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/loki/api/v1/push") {
		return base
	}
	return base + "/loki/api/v1/push"
}

var lokiLabelKeyPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func sanitizeLabelKey(v string) string {
	k := strings.TrimSpace(v)
	if k == "" || !lokiLabelKeyPattern.MatchString(k) {
		return ""
	}
	return k
}

func sanitizeLabelValue(v string) string {
	return strings.TrimSpace(v)
}
