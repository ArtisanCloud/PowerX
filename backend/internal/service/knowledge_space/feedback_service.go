package knowledge_space

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/instrumentation"
	workflow "github.com/ArtisanCloud/PowerX/internal/workflow/knowledge_space"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// FeedbackService orchestrates feedback intake and reprocessing.
type FeedbackService struct {
	db        *gorm.DB
	inst      *instrumentation.Instrumentation
	pipeline  workflow.ReprocessPipeline
	metrics   *IngestionMetricsWriter
	telemetry *FeedbackMetricsWriter
	clock     func() time.Time
}

// FeedbackServiceOptions configures runtime dependencies.
type FeedbackServiceOptions struct {
	DB              *gorm.DB
	Instrumentation *instrumentation.Instrumentation
	Pipeline        workflow.ReprocessPipeline
	MetricsWriter   *IngestionMetricsWriter
	FeedbackMetrics *FeedbackMetricsWriter
	Clock           func() time.Time
}

// SubmitFeedbackInput represents a feedback submission payload.
type SubmitFeedbackInput struct {
	SpaceID      uuid.UUID
	ReportedBy   string
	Severity     string
	IssueType    string
	Notes        string
	ToolTraceRef string
	LinkedChunks []uuid.UUID
}

// NewFeedbackService constructs a feedback service instance.
func NewFeedbackService(opts FeedbackServiceOptions) *FeedbackService {
	if opts.DB == nil {
		panic("feedback service requires db")
	}
	if opts.Instrumentation == nil {
		opts.Instrumentation = instrumentation.New(instrumentation.Options{})
	}
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	if opts.MetricsWriter == nil {
		opts.MetricsWriter = NewIngestionMetricsWriter(defaultMetricsPath)
	}
	if opts.FeedbackMetrics == nil {
		opts.FeedbackMetrics = NewFeedbackMetricsWriter(defaultFeedbackMetricsPath, defaultKnowledgeUpdatePath)
	}
	return &FeedbackService{
		db:        opts.DB,
		inst:      opts.Instrumentation,
		pipeline:  opts.Pipeline,
		metrics:   opts.MetricsWriter,
		telemetry: opts.FeedbackMetrics,
		clock:     opts.Clock,
	}
}

// SubmitFeedback creates a feedback case and schedules reprocessing.
func (s *FeedbackService) SubmitFeedback(ctx context.Context, in SubmitFeedbackInput) (*models.FeedbackCase, error) {
	if in.SpaceID == uuid.Nil || len(in.LinkedChunks) == 0 {
		return nil, ErrInvalidInput
	}
	severity := normalizeSeverity(in.Severity)
	issueType := normalizeIssueType(in.IssueType)
	reportedBy := strings.TrimSpace(in.ReportedBy)
	if reportedBy == "" {
		reportedBy = "ops@powerx.local"
	}
	chunkStrings := uniqueChunkStrings(in.LinkedChunks)
	if len(chunkStrings) == 0 {
		return nil, ErrInvalidInput
	}
	chunkPayload, _ := json.Marshal(chunkStrings)
	dueAt := s.clock().Add(slaWindow(severity))
	sanitizedNotes := sanitizeNotes(in.Notes)
	qualityScore := scoreQuality(severity, issueType)

	var created *models.FeedbackCase
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		spaces := repo.NewKnowledgeSpaceRepository(tx)
		cases := repo.NewFeedbackCaseRepository(tx)

		space, err := spaces.FindByUUID(ctx, in.SpaceID)
		if err != nil {
			return err
		}
		if space == nil || space.Status == models.KnowledgeSpaceStatusRetired {
			return ErrSpaceNotFound
		}

		caseModel := &models.FeedbackCase{
			SpaceUUID:    space.UUID,
			ReportedBy:   reportedBy,
			IssueType:    issueType,
			Severity:     severity,
			Status:       models.FeedbackStatusOpen,
			LinkedChunks: datatypes.JSON(chunkPayload),
			ToolTraceRef: in.ToolTraceRef,
			Notes:        sanitizedNotes,
			QualityScore: qualityScore,
			SLADueAt:     &dueAt,
		}

		caseModel, err = cases.Create(ctx, caseModel)
		if err != nil {
			return err
		}

		if s.pipeline != nil {
			task, err := s.pipeline.Schedule(ctx, workflow.ReprocessInput{
				SpaceID:     caseModel.SpaceUUID,
				CaseID:      caseModel.UUID,
				Severity:    severity,
				IssueType:   issueType,
				ChunkIDs:    in.LinkedChunks,
				RequestedBy: reportedBy,
			})
			if err != nil {
				return err
			}
			caseModel.Status = models.FeedbackStatusInProgress
			caseModel.ReprocessJobID = &task.JobID
			if err := tx.Model(caseModel).Updates(map[string]any{
				"status":           caseModel.Status,
				"reprocess_job_id": caseModel.ReprocessJobID,
			}).Error; err != nil {
				return err
			}
		}

		if err := s.writeAudit(ctx, tx, caseModel, "feedback.submitted", map[string]any{
			"severity":   severity,
			"issue_type": issueType,
			"chunks":     chunkStrings,
		}); err != nil {
			return err
		}

		created = caseModel
		return nil
	})
	if err != nil {
		return nil, err
	}

	snapshot := FeedbackSnapshot{
		SpaceID:       created.SpaceUUID.String(),
		CaseID:        created.UUID.String(),
		Severity:      created.Severity,
		Status:        created.Status,
		ReportedBy:    created.ReportedBy,
		IssueType:     created.IssueType,
		SLADueAt:      created.SLADueAt,
		LastSubmitted: &created.CreatedAt,
	}
	if created.ReprocessJobID != nil {
		snapshot.ReprocessJobID = *created.ReprocessJobID
	}

	openCases, err := repo.NewFeedbackCaseRepository(s.db).ListOpenBySpace(ctx, created.SpaceUUID)
	if err == nil {
		snapshot.OpenCases = len(openCases)
	}
	_ = s.metrics.StoreFeedback(snapshot)
	s.refreshFeedbackMetrics(ctx)
	return created, nil
}

func (s *FeedbackService) refreshFeedbackMetrics(ctx context.Context) {
	if s.telemetry == nil {
		return
	}
	if _, err := s.telemetry.Refresh(ctx, s.db); err != nil {
		s.inst.Logger(ctx).WarnF(ctx, "[feedback] refresh metrics failed: %v", err)
	}
}

// ListCases returns the latest feedback cases for a space.
func (s *FeedbackService) ListCases(ctx context.Context, space uuid.UUID, limit int) ([]*models.FeedbackCase, error) {
	if space == uuid.Nil {
		return nil, ErrInvalidInput
	}
	if limit <= 0 {
		limit = 50
	}
	var cases []*models.FeedbackCase
	if err := s.db.WithContext(ctx).
		Where("space_uuid = ?", space).
		Order("created_at DESC").
		Limit(limit).
		Find(&cases).Error; err != nil {
		return nil, err
	}
	return cases, nil
}

func (s *FeedbackService) writeAudit(ctx context.Context, tx *gorm.DB, caseModel *models.FeedbackCase, action string, payload map[string]any) error {
	if caseModel == nil {
		return errors.New("feedback case missing")
	}
	entry := &models.AuditTrailEntry{
		SpaceUUID:     caseModel.SpaceUUID,
		Action:        action,
		Actor:         caseModel.ReportedBy,
		Metadata:      marshalJSON(payload),
		OccurredAt:    s.clock(),
		RollbackToken: caseModel.UUID.String(),
		PayloadHash:   computePayloadHash(payload),
	}
	_, err := repo.NewAuditTrailRepository(tx).Create(ctx, entry)
	return err
}

func normalizeSeverity(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case models.FeedbackSeverityLow:
		return models.FeedbackSeverityLow
	case models.FeedbackSeverityHigh:
		return models.FeedbackSeverityHigh
	case models.FeedbackSeverityCritical:
		return models.FeedbackSeverityCritical
	default:
		return models.FeedbackSeverityMedium
	}
}

func normalizeIssueType(issueType string) string {
	switch strings.ToLower(strings.TrimSpace(issueType)) {
	case "freshness":
		return "freshness"
	case "compliance":
		return "compliance"
	default:
		return "accuracy"
	}
}

func uniqueChunkStrings(ids []uuid.UUID) []string {
	seen := make(map[string]struct{}, len(ids))
	var list []string
	for _, id := range ids {
		if id == uuid.Nil {
			continue
		}
		key := id.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		list = append(list, key)
	}
	sort.Strings(list)
	return list
}

func slaWindow(severity string) time.Duration {
	switch severity {
	case models.FeedbackSeverityCritical:
		return 12 * time.Hour
	case models.FeedbackSeverityHigh:
		return 24 * time.Hour
	case models.FeedbackSeverityLow:
		return 72 * time.Hour
	default:
		return 48 * time.Hour
	}
}

var piiEmailRegex = regexp.MustCompile(`[\w\.\-]+@[\w\.\-]+\.[A-Za-z]{2,}`)

func sanitizeNotes(notes string) string {
	if notes == "" {
		return notes
	}
	return piiEmailRegex.ReplaceAllString(notes, "[redacted]")
}

func scoreQuality(severity, issueType string) float64 {
	base := map[string]float64{
		models.FeedbackSeverityLow:      0.2,
		models.FeedbackSeverityMedium:   0.5,
		models.FeedbackSeverityHigh:     0.8,
		models.FeedbackSeverityCritical: 1.0,
	}[severity]
	if issueType == "compliance" {
		base += 0.1
	}
	if base > 1.0 {
		base = 1.0
	}
	return base
}
