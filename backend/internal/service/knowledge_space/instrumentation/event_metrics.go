package instrumentation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// EventMetricsSnapshot captures event hotfix telemetry.
type EventMetricsSnapshot struct {
	EventID        string    `json:"eventId"`
	EventType      string    `json:"eventType"`
	PolicySeverity string    `json:"policySeverity,omitempty"`
	Actions        []string  `json:"actions,omitempty"`
	LatencyMs      int64     `json:"latencyMs"`
	RetryCount     int       `json:"retryCount"`
	IdempotentSkip bool      `json:"idempotentSkip"`
	AgentRefreshOK bool      `json:"agentRefreshOk"`
	HotUpdateOK    bool      `json:"hotUpdateOk"`
	RecordedAt     time.Time `json:"recordedAt"`
}

// EventMetricsWriter persists snapshots and updates aggregate knowledge-update report.
type EventMetricsWriter struct {
	mu            sync.Mutex
	path          string
	aggregatePath string
}

func NewEventMetricsWriter(path, aggregate string) *EventMetricsWriter {
	return &EventMetricsWriter{path: path, aggregatePath: aggregate}
}

func (w *EventMetricsWriter) Store(snapshot EventMetricsSnapshot) error {
	if w == nil {
		return nil
	}
	if snapshot.RecordedAt.IsZero() {
		snapshot.RecordedAt = time.Now().UTC()
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.persistJSON(w.path, snapshot); err != nil {
		return err
	}
	return w.persistAggregate(snapshot)
}

func (w *EventMetricsWriter) persistJSON(path string, payload interface{}) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (w *EventMetricsWriter) persistAggregate(snapshot EventMetricsSnapshot) error {
	if strings.TrimSpace(w.aggregatePath) == "" {
		return nil
	}
	state := make(map[string]any)
	if raw, err := os.ReadFile(w.aggregatePath); err == nil {
		_ = json.Unmarshal(raw, &state)
	}
	state["event"] = snapshot
	return w.persistJSON(w.aggregatePath, state)
}
