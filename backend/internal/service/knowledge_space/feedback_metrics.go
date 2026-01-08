package knowledge_space

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"gorm.io/gorm"
)

const (
	defaultFeedbackMetricsPath = "backend/reports/_state/knowledge-feedback.json"
	defaultKnowledgeUpdatePath = "reports/_state/knowledge-update.json"
	defaultFeedbackLedgerName  = "knowledge-feedback-ledger.json"
	defaultKnowledgeSpacesPath = "backend/reports/_state/knowledge-spaces.json"
)

// FeedbackMetrics aggregates fleet-wide feedback signals for dashboards + audits.
type FeedbackMetrics struct {
	LoopTimeHours  float64   `json:"loopTimeHours"`
	FixAccuracyPct float64   `json:"fixAccuracyPct"`
	AutoRatePct    float64   `json:"autoRatePct"`
	Backlog        int       `json:"backlog"`
	SampleSize     int       `json:"sampleSize"`
	RecordedAt     time.Time `json:"recordedAt"`
}

// FeedbackMetricsWriter persists metrics snapshots + knowledge-update aggregate.
type FeedbackMetricsWriter struct {
	mu            sync.Mutex
	path          string
	aggregatePath string
}

// NewFeedbackMetricsWriter constructs a writer with sane defaults.
func NewFeedbackMetricsWriter(path, aggregate string) *FeedbackMetricsWriter {
	if strings.TrimSpace(path) == "" {
		path = defaultFeedbackMetricsPath
	}
	if strings.TrimSpace(aggregate) == "" {
		aggregate = defaultKnowledgeUpdatePath
	}
	return &FeedbackMetricsWriter{path: path, aggregatePath: aggregate}
}

// Refresh recomputes metrics from the database and persists snapshots.
func (w *FeedbackMetricsWriter) Refresh(ctx context.Context, db *gorm.DB) (FeedbackMetrics, error) {
	var metrics FeedbackMetrics
	if w == nil {
		return metrics, nil
	}
	if db == nil {
		return metrics, errors.New("feedback metrics writer requires db")
	}
	stats, err := w.compute(ctx, db)
	if err != nil {
		return metrics, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.persistSnapshot(w.path, stats); err != nil {
		return metrics, err
	}
	if err := w.persistLedger(ctx, db); err != nil {
		return metrics, err
	}
	if err := w.persistAggregate(stats); err != nil {
		return metrics, err
	}
	return stats, nil
}

func (w *FeedbackMetricsWriter) compute(ctx context.Context, db *gorm.DB) (FeedbackMetrics, error) {
	metrics := FeedbackMetrics{RecordedAt: time.Now().UTC()}
	var backlog int64
	if err := db.WithContext(ctx).
		Model(&models.FeedbackCase{}).
		Where("status IN ?", []string{models.FeedbackStatusOpen, models.FeedbackStatusInProgress}).
		Count(&backlog).Error; err != nil {
		return metrics, err
	}
	metrics.Backlog = int(backlog)

	var closedCases []models.FeedbackCase
	if err := db.WithContext(ctx).
		Order("updated_at DESC").
		Limit(200).
		Find(&closedCases).Error; err != nil {
		return metrics, err
	}
	var totalDuration float64
	for _, c := range closedCases {
		if c.Status != models.FeedbackStatusClosed && c.Status != models.FeedbackStatusReprocessed {
			continue
		}
		finishedAt := c.ClosedAt
		if finishedAt == nil {
			finishedAt = &c.UpdatedAt
		}
		delta := finishedAt.Sub(c.CreatedAt).Hours()
		if delta < 0 || math.IsInf(delta, 0) || math.IsNaN(delta) {
			continue
		}
		totalDuration += delta
		metrics.SampleSize++
	}
	if metrics.SampleSize > 0 {
		metrics.LoopTimeHours = totalDuration / float64(metrics.SampleSize)
	}

	var resolvedTotal int64
	if err := db.WithContext(ctx).
		Model(&models.FeedbackCase{}).
		Where("status IN ?", []string{models.FeedbackStatusReprocessed, models.FeedbackStatusClosed}).
		Count(&resolvedTotal).Error; err != nil {
		return metrics, err
	}
	if resolvedTotal > 0 {
		metrics.FixAccuracyPct = w.percent(ctx, db, "issue_type = ?", []any{"accuracy"}, resolvedTotal)
		metrics.AutoRatePct = w.percent(ctx, db, "reprocess_job_id IS NOT NULL", nil, resolvedTotal)
	}
	return metrics, nil
}

func (w *FeedbackMetricsWriter) percent(ctx context.Context, db *gorm.DB, clause string, args []any, base int64) float64 {
	var matched int64
	conditions := db.WithContext(ctx).
		Model(&models.FeedbackCase{}).
		Where("status IN ?", []string{models.FeedbackStatusReprocessed, models.FeedbackStatusClosed})
	if clause != "" {
		conditions = conditions.Where(clause, args...)
	}
	if err := conditions.Count(&matched).Error; err != nil || base == 0 {
		return 0
	}
	return math.Round((float64(matched)/float64(base))*1000) / 10
}

func (w *FeedbackMetricsWriter) persistSnapshot(path string, metrics FeedbackMetrics) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	buf, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, buf, 0o644)
}

func (w *FeedbackMetricsWriter) persistAggregate(metrics FeedbackMetrics) error {
	if strings.TrimSpace(w.aggregatePath) == "" {
		return nil
	}
	state := make(map[string]any)
	if data, err := os.ReadFile(w.aggregatePath); err == nil {
		_ = json.Unmarshal(data, &state)
	}
	state["feedback"] = metrics
	if err := os.MkdirAll(filepath.Dir(w.aggregatePath), 0o755); err != nil {
		return err
	}
	buf, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(w.aggregatePath, buf, 0o644)
}

func (w *FeedbackMetricsWriter) persistLedger(ctx context.Context, db *gorm.DB) error {
	if strings.TrimSpace(w.path) == "" {
		return nil
	}
	dir := filepath.Dir(w.path)
	ledgerPath := filepath.Join(dir, defaultFeedbackLedgerName)

	var cases []models.FeedbackCase
	if err := db.WithContext(ctx).
		Order("created_at DESC").
		Limit(100).
		Find(&cases).Error; err != nil {
		return err
	}
	type caseView struct {
		CaseID       string     `json:"case_id"`
		SpaceID      string     `json:"space_id"`
		Status       string     `json:"status"`
		Severity     string     `json:"severity"`
		IssueType    string     `json:"issue_type"`
		TraceID      string     `json:"trace_id,omitempty"`
		ReprocessJob *uint64    `json:"reprocess_job_id,omitempty"`
		SLADueAt     *time.Time `json:"sla_due_at,omitempty"`
		EscalatedAt  *time.Time `json:"escalated_at,omitempty"`
		ClosedAt     *time.Time `json:"closed_at,omitempty"`
		CreatedAt    time.Time  `json:"created_at"`
		UpdatedAt    time.Time  `json:"updated_at"`
	}
	views := make([]caseView, 0, len(cases))
	for _, c := range cases {
		views = append(views, caseView{
			CaseID:       c.UUID.String(),
			SpaceID:      c.SpaceUUID.String(),
			Status:       c.Status,
			Severity:     c.Severity,
			IssueType:    c.IssueType,
			TraceID:      c.ToolTraceRef,
			ReprocessJob: c.ReprocessJobID,
			SLADueAt:     c.SLADueAt,
			EscalatedAt:  c.EscalatedAt,
			ClosedAt:     c.ClosedAt,
			CreatedAt:    c.CreatedAt,
			UpdatedAt:    c.UpdatedAt,
		})
	}

	var audits []models.AuditTrailEntry
	if err := db.WithContext(ctx).
		Where("action LIKE ?", "feedback.%").
		Order("occurred_at DESC").
		Limit(200).
		Find(&audits).Error; err != nil {
		return err
	}
	type auditView struct {
		ID        uint64    `json:"id"`
		SpaceID   string    `json:"space_id"`
		Action    string    `json:"action"`
		Actor     string    `json:"actor"`
		Hash      string    `json:"payload_hash"`
		Occurred  time.Time `json:"occurred_at"`
		Token     string    `json:"rollback_token,omitempty"`
		Metadata  any       `json:"metadata,omitempty"`
	}
	auditViews := make([]auditView, 0, len(audits))
	for _, a := range audits {
		var meta any
		if len(a.Metadata) > 0 {
			_ = json.Unmarshal(a.Metadata, &meta)
		}
		auditViews = append(auditViews, auditView{
			ID:       a.ID,
			SpaceID:  a.SpaceUUID.String(),
			Action:   a.Action,
			Actor:    a.Actor,
			Hash:     a.PayloadHash,
			Occurred: a.OccurredAt,
			Token:    a.RollbackToken,
			Metadata: meta,
		})
	}

	payload := map[string]any{
		"recorded_at": time.Now().UTC(),
		"cases":       views,
		"audits":      auditViews,
	}
	if err := os.MkdirAll(filepath.Dir(ledgerPath), 0o755); err != nil {
		return err
	}
	buf, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(ledgerPath, buf, 0o644); err != nil {
		return err
	}

	// Optional: update knowledge-spaces.json if present.
	if _, err := os.Stat(defaultKnowledgeSpacesPath); err == nil {
		state := make(map[string]any)
		if data, err := os.ReadFile(defaultKnowledgeSpacesPath); err == nil {
			_ = json.Unmarshal(data, &state)
		}
		state["feedback"] = payload
		buf, err := json.MarshalIndent(state, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(defaultKnowledgeSpacesPath, buf, 0o644); err != nil {
			return err
		}
	}
	return nil
}
