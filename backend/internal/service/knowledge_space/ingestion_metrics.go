package knowledge_space

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const defaultMetricsPath = "reports/_state/knowledge-spaces.json"

// IngestionSnapshot captures lightweight job stats for reporting.
type IngestionSnapshot struct {
	SpaceID             string     `json:"spaceId"`
	JobID               string     `json:"jobId"`
	ChunkTotal          int        `json:"chunkTotal"`
	SummaryChunkCount   int        `json:"summaryChunkCount"`
	ParagraphChunkCount int        `json:"paragraphChunkCount"`
	CoveragePct         float64    `json:"coveragePct"`
	EmbeddingPct        float64    `json:"embeddingPct"`
	MaskingPct          float64    `json:"maskingPct"`
	CompletedAt         *time.Time `json:"completedAt"`
}

// IngestionMetricsWriter maintains a JSON snapshot file.
type IngestionMetricsWriter struct {
	path string
	mu   sync.Mutex
}

func NewIngestionMetricsWriter(path string) *IngestionMetricsWriter {
	if path == "" {
		path = defaultMetricsPath
	}
	return &IngestionMetricsWriter{path: path}
}

// Store upserts the snapshot for the corresponding space ID.
func (w *IngestionMetricsWriter) Store(snapshot IngestionSnapshot) error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	state := make(map[string]IngestionSnapshot)
	if existing, err := os.ReadFile(w.path); err == nil {
		_ = json.Unmarshal(existing, &state)
	}
	state[snapshot.SpaceID] = snapshot

	if err := os.MkdirAll(filepath.Dir(w.path), 0o755); err != nil {
		return err
	}
	buf, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(w.path, buf, 0o644)
}
