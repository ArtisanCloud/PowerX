package instrumentation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DecayMetricsSnapshot captures decay/ gap metrics.
type DecayMetricsSnapshot struct {
	Detected          int       `json:"detected"`
	FalsePositive     int       `json:"falsePositive"`
	Backlog           int       `json:"backlog"`
	AverageFillHours  float64   `json:"avgFillHours"`
	Metrics           map[string]any `json:"metrics,omitempty"`
	RecordedAt        time.Time `json:"recordedAt"`
}

func (s *DecayMetricsSnapshot) EnsureMetrics() {
	if s == nil {
		return
	}
	if s.Metrics == nil {
		s.Metrics = make(map[string]any)
	}
	s.Metrics["knowledge.decay.detected"] = s.Detected
	s.Metrics["knowledge.decay.false_positive"] = s.FalsePositive
	s.Metrics["knowledge.gap.backlog"] = s.Backlog
	s.Metrics["knowledge.decay.fill_time_hours"] = s.AverageFillHours
}

// DecayMetricsWriter persists JSON snapshots and updates aggregates.
type DecayMetricsWriter struct {
	mu            sync.Mutex
	path          string
	aggregatePath string
}

func NewDecayMetricsWriter(path, aggregate string) *DecayMetricsWriter {
	return &DecayMetricsWriter{path: path, aggregatePath: aggregate}
}

func (w *DecayMetricsWriter) Store(snapshot DecayMetricsSnapshot) error {
	if w == nil {
		return nil
	}
	if snapshot.RecordedAt.IsZero() {
		snapshot.RecordedAt = time.Now().UTC()
	}
	snapshot.EnsureMetrics()
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.persistJSON(w.path, snapshot); err != nil {
		return err
	}
	return w.persistAggregate(snapshot)
}

func (w *DecayMetricsWriter) persistJSON(path string, payload interface{}) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	buf, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, buf, 0o644)
}

func (w *DecayMetricsWriter) persistAggregate(snapshot DecayMetricsSnapshot) error {
	if strings.TrimSpace(w.aggregatePath) == "" {
		return nil
	}
	state := make(map[string]any)
	if data, err := os.ReadFile(w.aggregatePath); err == nil {
		_ = json.Unmarshal(data, &state)
	}
	state["decay"] = snapshot
	return w.persistJSON(w.aggregatePath, state)
}
