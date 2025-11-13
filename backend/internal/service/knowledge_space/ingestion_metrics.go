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

// FeedbackSnapshot captures feedback loop state.
type FeedbackSnapshot struct {
	SpaceID        string     `json:"spaceId"`
	CaseID         string     `json:"caseId"`
	Severity       string     `json:"severity"`
	Status         string     `json:"status"`
	ReportedBy     string     `json:"reportedBy"`
	IssueType      string     `json:"issueType"`
	OpenCases      int        `json:"openCases"`
	SLADueAt       *time.Time `json:"slaDueAt,omitempty"`
	LastSubmitted  *time.Time `json:"lastSubmittedAt,omitempty"`
	ReprocessJobID uint64     `json:"reprocessJobId,omitempty"`
}

type knowledgeSpaceState struct {
	Ingestion *IngestionSnapshot `json:"ingestion,omitempty"`
	Feedback  *FeedbackSnapshot  `json:"feedback,omitempty"`
}

// IngestionMetricsWriter maintains a JSON snapshot file shared across ingestion/feedback.
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

// Store upserts the ingestion snapshot for the corresponding space ID.
func (w *IngestionMetricsWriter) Store(snapshot IngestionSnapshot) error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	state := w.loadState()
	entry := state[snapshot.SpaceID]
	copy := snapshot
	entry.Ingestion = &copy
	if entry.Ingestion.SpaceID == "" {
		entry.Ingestion.SpaceID = snapshot.SpaceID
	}
	state[snapshot.SpaceID] = entry
	return w.persistState(state)
}

// StoreFeedback upserts feedback stats for a space.
func (w *IngestionMetricsWriter) StoreFeedback(snapshot FeedbackSnapshot) error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	state := w.loadState()
	entry := state[snapshot.SpaceID]
	copy := snapshot
	if copy.SpaceID == "" {
		copy.SpaceID = snapshot.SpaceID
	}
	entry.Feedback = &copy
	state[snapshot.SpaceID] = entry
	return w.persistState(state)
}

func (w *IngestionMetricsWriter) loadState() map[string]knowledgeSpaceState {
	state := make(map[string]knowledgeSpaceState)
	if w == nil {
		return state
	}
	bytes, err := os.ReadFile(w.path)
	if err != nil {
		return state
	}
	// Attempt native format first.
	var typed map[string]knowledgeSpaceState
	if err := json.Unmarshal(bytes, &typed); err == nil {
		return typed
	}
	// Fallback to legacy ingestion-only format.
	var legacy map[string]IngestionSnapshot
	if err := json.Unmarshal(bytes, &legacy); err == nil {
		for k, v := range legacy {
			copy := v
			state[k] = knowledgeSpaceState{Ingestion: &copy}
		}
	}
	return state
}

func (w *IngestionMetricsWriter) persistState(state map[string]knowledgeSpaceState) error {
	if err := os.MkdirAll(filepath.Dir(w.path), 0o755); err != nil {
		return err
	}
	buf, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(w.path, buf, 0o644)
}
