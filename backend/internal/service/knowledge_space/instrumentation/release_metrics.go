package instrumentation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ReleaseMetricsSnapshot captures tenant release status.
type ReleaseMetricsSnapshot struct {
	GrayState      string    `json:"grayState"`
	RollbackCount  int       `json:"rollbackCount"`
	TenantCoverage float64   `json:"tenantCoverage"`
	VersionDrift   int       `json:"versionDrift"`
	Alerts         []string  `json:"alerts"`
	Metrics        map[string]any `json:"metrics,omitempty"`
	RecordedAt     time.Time `json:"recordedAt"`
}

func (s *ReleaseMetricsSnapshot) EnsureMetrics() {
	if s == nil {
		return
	}
	if s.Metrics == nil {
		s.Metrics = make(map[string]any)
	}
	s.Metrics["knowledge.release.gray_state"] = s.GrayState
	s.Metrics["knowledge.release.rollback_count"] = s.RollbackCount
	s.Metrics["knowledge.release.tenant_coverage"] = s.TenantCoverage
	s.Metrics["knowledge.release.version_drift"] = s.VersionDrift
	s.Metrics["knowledge.release.alerts"] = append([]string{}, s.Alerts...)
}

// ReleaseMetricsWriter persists release telemetry and aggregates.
type ReleaseMetricsWriter struct {
	mu            sync.Mutex
	path          string
	aggregatePath string
}

func NewReleaseMetricsWriter(path, aggregate string) *ReleaseMetricsWriter {
	return &ReleaseMetricsWriter{path: path, aggregatePath: aggregate}
}

func (w *ReleaseMetricsWriter) Store(snapshot ReleaseMetricsSnapshot) error {
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

func (w *ReleaseMetricsWriter) persistJSON(path string, payload interface{}) error {
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

func (w *ReleaseMetricsWriter) persistAggregate(snapshot ReleaseMetricsSnapshot) error {
	if strings.TrimSpace(w.aggregatePath) == "" {
		return nil
	}
	state := make(map[string]any)
	if data, err := os.ReadFile(w.aggregatePath); err == nil {
		_ = json.Unmarshal(data, &state)
	}
	state["release"] = snapshot
	return w.persistJSON(w.aggregatePath, state)
}
