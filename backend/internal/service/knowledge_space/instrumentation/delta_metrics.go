package instrumentation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DeltaMetricsSnapshot captures delta sync performance counters.
type DeltaMetricsSnapshot struct {
	JobID           string    `json:"jobId"`
	SpaceID         string    `json:"spaceId"`
	Status          string    `json:"status"`
	SLAMinutes      float64   `json:"slaMinutes"`
	ApprovalMinutes float64   `json:"approvalMinutes"`
	DiffAccuracyPct float64   `json:"diffAccuracyPct"`
	RollbackCount   int       `json:"rollbackCount"`
	PartialRelease  bool      `json:"partialRelease"`
	RecordedAt      time.Time `json:"recordedAt"`
}

// DeltaMetricsWriter persists per-job snapshots and updates the aggregate dashboard file.
type DeltaMetricsWriter struct {
	mu            sync.Mutex
	path          string
	aggregatePath string
}

// NewDeltaMetricsWriter constructs a writer using the provided paths.
func NewDeltaMetricsWriter(path, aggregate string) *DeltaMetricsWriter {
	return &DeltaMetricsWriter{path: path, aggregatePath: aggregate}
}

// Store writes the latest snapshot and updates the aggregate report.
func (w *DeltaMetricsWriter) Store(snapshot DeltaMetricsSnapshot) error {
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

func (w *DeltaMetricsWriter) persistJSON(path string, payload interface{}) error {
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

func (w *DeltaMetricsWriter) persistAggregate(snapshot DeltaMetricsSnapshot) error {
	if strings.TrimSpace(w.aggregatePath) == "" {
		return nil
	}
	state := make(map[string]any)
	if data, err := os.ReadFile(w.aggregatePath); err == nil {
		_ = json.Unmarshal(data, &state)
	}
	state["delta"] = snapshot
	return w.persistJSON(w.aggregatePath, state)
}
