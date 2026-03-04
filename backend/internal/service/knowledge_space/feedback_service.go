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
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
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

type FeedbackCitation struct {
	ChunkID  uuid.UUID         `json:"chunkId"`
	Citation map[string]any    `json:"citation,omitempty"`
	Meta     map[string]string `json:"meta,omitempty"`
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
		opts.MetricsWriter = NewIngestionMetricsWriter("")
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
	traceRef := strings.TrimSpace(in.ToolTraceRef)
	if traceRef == "" {
		traceRef = strings.TrimSpace(reqctx.GetTraceID(ctx))
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
			ToolTraceRef: traceRef,
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

		if err := s.writeAudit(ctx, tx, caseModel, "feedback.submitted", reportedBy, map[string]any{
			"severity":   severity,
			"issue_type": issueType,
			"chunks":     chunkStrings,
			"trace_id":   traceRef,
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

type ListFeedbackFilter struct {
	Status   string
	Severity string
	Limit    int
}

func (s *FeedbackService) ListCasesFiltered(ctx context.Context, space uuid.UUID, filter ListFeedbackFilter) ([]*models.FeedbackCase, error) {
	if space == uuid.Nil {
		return nil, ErrInvalidInput
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	query := s.db.WithContext(ctx).Where("space_uuid = ?", space)
	if status := strings.TrimSpace(filter.Status); status != "" {
		query = query.Where("status = ?", status)
	}
	if severity := strings.TrimSpace(filter.Severity); severity != "" {
		query = query.Where("severity = ?", severity)
	}
	var cases []*models.FeedbackCase
	if err := query.Order("created_at DESC").Limit(limit).Find(&cases).Error; err != nil {
		return nil, err
	}
	return cases, nil
}

type FeedbackCaseUpdateInput struct {
	SpaceID uuid.UUID
	CaseID  uuid.UUID
	Actor   string
	Notes   string
}

func (s *FeedbackService) CloseCase(ctx context.Context, in FeedbackCaseUpdateInput) (*models.FeedbackCase, error) {
	if in.SpaceID == uuid.Nil || in.CaseID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	actor := strings.TrimSpace(in.Actor)
	if actor == "" {
		actor = "ops@powerx.local"
	}
	now := s.clock()
	var updated *models.FeedbackCase
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
		caseModel, err := cases.GetByUUID(ctx, in.CaseID.String(), nil)
		if err != nil {
			return err
		}
		if caseModel == nil || caseModel.SpaceUUID != in.SpaceID {
			return ErrInvalidInput
		}
		caseModel.Status = models.FeedbackStatusClosed
		caseModel.ClosedAt = &now
		caseModel.ResolutionNotes = sanitizeNotes(in.Notes)
		if _, err := cases.Update(ctx, caseModel); err != nil {
			return err
		}
		if err := s.writeAudit(ctx, tx, caseModel, "feedback.closed", actor, map[string]any{
			"resolution_notes": caseModel.ResolutionNotes,
		}); err != nil {
			return err
		}
		updated = caseModel
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.refreshFeedbackMetrics(ctx)
	return updated, nil
}

func (s *FeedbackService) EscalateCase(ctx context.Context, in FeedbackCaseUpdateInput) (*models.FeedbackCase, error) {
	if in.SpaceID == uuid.Nil || in.CaseID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	actor := strings.TrimSpace(in.Actor)
	if actor == "" {
		actor = "ops@powerx.local"
	}
	now := s.clock()
	var updated *models.FeedbackCase
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
		caseModel, err := cases.GetByUUID(ctx, in.CaseID.String(), nil)
		if err != nil {
			return err
		}
		if caseModel == nil || caseModel.SpaceUUID != in.SpaceID {
			return ErrInvalidInput
		}
		caseModel.Status = models.FeedbackStatusEscalated
		caseModel.EscalatedAt = &now
		caseModel.ResolutionNotes = sanitizeNotes(in.Notes)
		if _, err := cases.Update(ctx, caseModel); err != nil {
			return err
		}
		if err := s.writeAudit(ctx, tx, caseModel, "feedback.escalated", actor, map[string]any{
			"reason": caseModel.ResolutionNotes,
		}); err != nil {
			return err
		}
		updated = caseModel
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.refreshFeedbackMetrics(ctx)
	return updated, nil
}

func (s *FeedbackService) ReprocessCase(ctx context.Context, spaceID, caseID uuid.UUID, requestedBy string) (*models.FeedbackCase, error) {
	if spaceID == uuid.Nil || caseID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	requestedBy = strings.TrimSpace(requestedBy)
	if requestedBy == "" {
		requestedBy = "ops@powerx.local"
	}
	var updated *models.FeedbackCase
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		spaces := repo.NewKnowledgeSpaceRepository(tx)
		cases := repo.NewFeedbackCaseRepository(tx)
		space, err := spaces.FindByUUID(ctx, spaceID)
		if err != nil {
			return err
		}
		if space == nil || space.Status == models.KnowledgeSpaceStatusRetired {
			return ErrSpaceNotFound
		}
		caseModel, err := cases.GetByUUID(ctx, caseID.String(), nil)
		if err != nil {
			return err
		}
		if caseModel == nil || caseModel.SpaceUUID != spaceID {
			return ErrInvalidInput
		}
		chunks := decodeChunkStrings(caseModel.LinkedChunks)
		chunkIDs := make([]uuid.UUID, 0, len(chunks))
		for _, raw := range chunks {
			id, err := uuid.Parse(raw)
			if err != nil {
				continue
			}
			chunkIDs = append(chunkIDs, id)
		}
		if s.pipeline == nil {
			return errors.New("reprocess pipeline not configured")
		}
		task, err := s.pipeline.Schedule(ctx, workflow.ReprocessInput{
			SpaceID:     spaceID,
			CaseID:      caseID,
			Severity:    caseModel.Severity,
			IssueType:   caseModel.IssueType,
			ChunkIDs:    chunkIDs,
			RequestedBy: requestedBy,
		})
		if err != nil {
			return err
		}
		caseModel.Status = models.FeedbackStatusInProgress
		caseModel.ReprocessJobID = &task.JobID
		if _, err := cases.Update(ctx, caseModel); err != nil {
			return err
		}
		if err := s.writeAudit(ctx, tx, caseModel, "feedback.reprocess.requested", requestedBy, map[string]any{
			"job_id": task.JobID,
		}); err != nil {
			return err
		}
		updated = caseModel
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.refreshFeedbackMetrics(ctx)
	return updated, nil
}

func (s *FeedbackService) RollbackCase(ctx context.Context, spaceID, caseID uuid.UUID, requestedBy string, reason string) (*models.FeedbackCase, error) {
	if spaceID == uuid.Nil || caseID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	requestedBy = strings.TrimSpace(requestedBy)
	if requestedBy == "" {
		requestedBy = "ops@powerx.local"
	}
	now := s.clock()
	var updated *models.FeedbackCase
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		spaces := repo.NewKnowledgeSpaceRepository(tx)
		cases := repo.NewFeedbackCaseRepository(tx)
		jobs := repo.NewIngestionJobRepository(tx)
		bundles := repo.NewArtifactBundleRepository(tx)

		space, err := spaces.FindByUUID(ctx, spaceID)
		if err != nil {
			return err
		}
		if space == nil || space.Status == models.KnowledgeSpaceStatusRetired {
			return ErrSpaceNotFound
		}
		caseModel, err := cases.GetByUUID(ctx, caseID.String(), nil)
		if err != nil {
			return err
		}
		if caseModel == nil || caseModel.SpaceUUID != spaceID {
			return ErrInvalidInput
		}

		var recent []models.IngestionJob
		if err := tx.WithContext(ctx).
			Model(&models.IngestionJob{}).
			Where("space_uuid = ? AND artifact_bundle_id IS NOT NULL", spaceID).
			Order("created_at DESC").
			Limit(2).
			Find(&recent).Error; err != nil {
			return err
		}
		if len(recent) < 2 || recent[0].ArtifactBundleID == nil || recent[1].ArtifactBundleID == nil {
			return ErrInvalidInput
		}
		currentBundleID := *recent[0].ArtifactBundleID
		prevBundleID := *recent[1].ArtifactBundleID

		currentBundle, err := bundles.GetById(ctx, currentBundleID, nil)
		if err != nil {
			return err
		}
		prevBundle, err := bundles.GetById(ctx, prevBundleID, nil)
		if err != nil {
			return err
		}
		if currentBundle == nil || prevBundle == nil {
			return ErrInvalidInput
		}

		currentBundle.Status = models.ArtifactBundleStatusArchived
		if _, err := bundles.Update(ctx, currentBundle); err != nil {
			return err
		}
		prevBundle.Status = models.ArtifactBundleStatusActive
		if _, err := bundles.Update(ctx, prevBundle); err != nil {
			return err
		}

		job, err := jobs.GetByUUID(ctx, recent[0].UUID.String(), nil)
		if err == nil && job != nil {
			job.Status = models.IngestionStatusFailed
			job.ErrorCode = "REPROCESS_ROLLED_BACK"
			job.BlockedReason = sanitizeNotes(reason)
			job.CompletedAt = &now
			_, _ = jobs.Update(ctx, job)
		}

		caseModel.Status = models.FeedbackStatusClosed
		caseModel.ClosedAt = &now
		caseModel.ResolutionNotes = sanitizeNotes("rollback: " + reason)
		if _, err := cases.Update(ctx, caseModel); err != nil {
			return err
		}
		if err := s.writeAudit(ctx, tx, caseModel, "feedback.rollback", requestedBy, map[string]any{
			"bundle_current":  currentBundleID,
			"bundle_previous": prevBundleID,
			"reason":          sanitizeNotes(reason),
		}); err != nil {
			return err
		}
		updated = caseModel
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.refreshFeedbackMetrics(ctx)
	return updated, nil
}

type FeedbackExport struct {
	Cases  []*models.FeedbackCase     `json:"cases"`
	Audits []*models.AuditTrailEntry  `json:"audits"`
	Meta   map[string]any             `json:"meta,omitempty"`
}

func (s *FeedbackService) ExportCases(ctx context.Context, space uuid.UUID, filter ListFeedbackFilter) (*FeedbackExport, error) {
	if space == uuid.Nil {
		return nil, ErrInvalidInput
	}
	cases, err := s.ListCasesFiltered(ctx, space, filter)
	if err != nil {
		return nil, err
	}
	var audits []*models.AuditTrailEntry
	if err := s.db.WithContext(ctx).
		Where("space_uuid = ?", space).
		Order("occurred_at DESC").
		Limit(200).
		Find(&audits).Error; err != nil {
		return nil, err
	}
	return &FeedbackExport{
		Cases:  cases,
		Audits: audits,
		Meta: map[string]any{
			"space_id":    space.String(),
			"exported_at": time.Now().UTC(),
		},
	}, nil
}

func (s *FeedbackService) writeAudit(ctx context.Context, tx *gorm.DB, caseModel *models.FeedbackCase, action string, actor string, payload map[string]any) error {
	if caseModel == nil {
		return errors.New("feedback case missing")
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = caseModel.ReportedBy
	}
	entry := &models.AuditTrailEntry{
		SpaceUUID:     caseModel.SpaceUUID,
		Action:        action,
		Actor:         actor,
		Metadata:      marshalJSON(payload),
		OccurredAt:    s.clock(),
		RollbackToken: caseModel.UUID.String(),
		PayloadHash:   computePayloadHash(payload),
	}
	_, err := repo.NewAuditTrailRepository(tx).Create(ctx, entry)
	return err
}

func decodeChunkStrings(raw datatypes.JSON) []string {
	if len(raw) == 0 {
		return nil
	}
	var chunks []string
	if err := json.Unmarshal(raw, &chunks); err != nil {
		return nil
	}
	return chunks
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
