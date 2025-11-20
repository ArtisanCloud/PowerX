package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/instrumentation"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_release"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Domain errors surfaced to transports.
var (
	ErrInvalidInput      = errors.New("plugin_release.pipeline: invalid input")
	ErrCandidateNotFound = errors.New("plugin_release.pipeline: candidate not found")
	ErrPlanWindowInvalid = errors.New("plugin_release.pipeline: invalid release window")
	ErrGateNotPassed     = errors.New("plugin_release.pipeline: quality gates not passed")
)

// Service orchestrates release candidate submission, gate evaluation and plan generation.
type Service struct {
	candidates  *repo.ReleaseCandidateRepository
	plans       *repo.ReleasePlanRepository
	gates       *GateRunner
	hooks       AuditHooks
	instruments *instrumentation.Instruments
	clock       func() time.Time
}

// Options configures pipeline service behaviour.
type Options struct {
	Hooks       AuditHooks
	Instruments *instrumentation.Instruments
	Clock       func() time.Time
}

// SubmitCandidateInput captures metadata submitted by CLI/API.
type SubmitCandidateInput struct {
	TenantID        string
	PluginID        string
	Version         string
	BuildArtifact   string
	CommitHash      string
	ReleaseNotes    string
	Labels          map[string]string
	Actor           string
	ApprovalContext string
}

// UpdateCandidateInput enumerates mutable fields on an existing candidate.
type UpdateCandidateInput struct {
	CandidateID   uuid.UUID
	BuildArtifact string
	ReleaseNotes  string
	Labels        map[string]string
	Actor         string
}

// RunQualityGatesInput describes a gate evaluation request.
type RunQualityGatesInput struct {
	CandidateID uuid.UUID
	Actor       string
	ForceRescan bool
}

// GeneratePlanInput contains desired production rollout configuration.
type GeneratePlanInput struct {
	CandidateID         uuid.UUID
	WindowStart         time.Time
	WindowEnd           time.Time
	CanaryBatches       []CanaryBatchInput
	RollbackScripts     []string
	NotificationTargets []string
	DependencyList      []string
	Actor               string
}

// CanaryBatchInput mirrors API payload for rollout batches.
type CanaryBatchInput struct {
	Name                string             `json:"name"`
	TenantScope         []string           `json:"tenantScope"`
	MetricThresholds    map[string]float64 `json:"metricThresholds"`
	RollbackTimeoutMins uint32             `json:"rollbackTimeoutMinutes"`
}

// GateResult summarises gate execution.
type GateResult struct {
	CandidateID uuid.UUID
	Status      string
	Violations  []GateViolation
}

// NewService wires repositories with gate runner and audit hooks.
func NewService(
	candidates *repo.ReleaseCandidateRepository,
	plans *repo.ReleasePlanRepository,
	gates *GateRunner,
	opts Options,
) *Service {
	if gates == nil {
		gates = NewGateRunner(GateRunnerOptions{})
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	hooks := opts.Hooks
	if hooks == nil {
		hooks = NoopAuditHooks{}
	}
	return &Service{
		candidates:  candidates,
		plans:       plans,
		gates:       gates,
		hooks:       hooks,
		instruments: opts.Instruments,
		clock:       clock,
	}
}

// SubmitCandidate validates input and upserts a release candidate.
func (s *Service) SubmitCandidate(ctx context.Context, input SubmitCandidateInput) (*models.PluginReleaseCandidate, error) {
	if err := validateCandidateInput(input); err != nil {
		return nil, err
	}
	now := s.clock()
	candidate := &models.PluginReleaseCandidate{
		TenantID:         input.TenantID,
		PluginID:         input.PluginID,
		Version:          input.Version,
		BuildArtifactURI: input.BuildArtifact,
		CommitHash:       input.CommitHash,
		ReleaseNotes:     input.ReleaseNotes,
		GateStatus:       models.PluginReleaseGateStatusPending,
		ApprovalStatus:   models.PluginReleaseApprovalSubmitted,
		SubmittedAt:      &now,
		CreatedBy:        input.Actor,
		UpdatedBy:        input.Actor,
	}
	labelsJSON, err := encodeJSON(input.Labels)
	if err != nil {
		return nil, err
	}
	candidate.Labels = labelsJSON

	result, err := s.candidates.CreateCandidate(ctx, candidate)
	if err != nil {
		return nil, err
	}
	s.hooks.CandidateSubmitted(ctx, result)
	return result, nil
}

// RunQualityGates executes the gate runner and persists status.
func (s *Service) RunQualityGates(ctx context.Context, input RunQualityGatesInput) (*GateResult, error) {
	if input.CandidateID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	candidate, err := s.candidates.GetByUUID(ctx, input.CandidateID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCandidateNotFound
		}
		return nil, err
	}
	if candidate == nil {
		return nil, ErrCandidateNotFound
	}
	report := s.gates.Run(ctx, candidate)
	now := s.clock()
	candidate.GatesCheckedAt = &now
	if report.Passed {
		candidate.GateStatus = models.PluginReleaseGateStatusPassed
		candidate.FailureReason = ""
	} else {
		candidate.GateStatus = models.PluginReleaseGateStatusFailed
		if len(report.Violations) > 0 {
			candidate.FailureReason = report.Violations[0].Message
		}
	}
	if len(report.Score) > 0 {
		if scoreJSON, err := encodeJSON(report.Score); err == nil {
			candidate.ScanScore = scoreJSON
		}
	}
	updateFields := map[string]interface{}{
		"gate_status":      candidate.GateStatus,
		"failure_reason":   candidate.FailureReason,
		"gates_checked_at": candidate.GatesCheckedAt,
		"updated_at":       now,
	}
	if len(candidate.ScanScore) > 0 {
		updateFields["scan_score"] = candidate.ScanScore
	}
	if err := s.candidates.UpdateFieldsByUUID(ctx, candidate.UUID, updateFields); err != nil {
		return nil, err
	}
	result := &GateResult{
		CandidateID: candidate.UUID,
		Status:      candidate.GateStatus,
		Violations:  report.Violations,
	}
	s.hooks.GatesEvaluated(ctx, candidate, *result)
	return result, nil
}

// GenerateReleasePlan persists rollout instructions for an approved candidate.
func (s *Service) GenerateReleasePlan(ctx context.Context, input GeneratePlanInput) (*models.ReleasePlan, *models.PluginReleaseCandidate, error) {
	if input.CandidateID == uuid.Nil {
		return nil, nil, ErrInvalidInput
	}
	if input.WindowEnd.Before(input.WindowStart) || input.WindowEnd.Equal(input.WindowStart) {
		return nil, nil, ErrPlanWindowInvalid
	}
	candidate, err := s.candidates.GetByUUID(ctx, input.CandidateID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrCandidateNotFound
		}
		return nil, nil, err
	}
	if candidate == nil {
		return nil, nil, ErrCandidateNotFound
	}
	if candidate.GateStatus != models.PluginReleaseGateStatusPassed {
		return nil, nil, ErrGateNotPassed
	}

	canaryJSON, err := encodeJSON(input.CanaryBatches)
	if err != nil {
		return nil, nil, err
	}
	rollbackJSON, err := encodeJSON(input.RollbackScripts)
	if err != nil {
		return nil, nil, err
	}
	notificationJSON, err := encodeJSON(input.NotificationTargets)
	if err != nil {
		return nil, nil, err
	}
	dependencyJSON, err := encodeJSON(input.DependencyList)
	if err != nil {
		return nil, nil, err
	}

	plan := &models.ReleasePlan{
		ReleaseCandidateID:  candidate.ID,
		WindowStart:         input.WindowStart.UTC(),
		WindowEnd:           input.WindowEnd.UTC(),
		CanaryBatches:       canaryJSON,
		RollbackScripts:     rollbackJSON,
		NotificationTargets: notificationJSON,
		DependencyList:      dependencyJSON,
		Status:              models.ReleasePlanStatusDraft,
		CreatedBy:           input.Actor,
		UpdatedBy:           input.Actor,
	}

	dbCanaryRecords := make([]*models.CanaryDeploymentRecord, 0, len(input.CanaryBatches))
	for _, batch := range input.CanaryBatches {
		scopeJSON, err := encodeJSON(batch.TenantScope)
		if err != nil {
			return nil, nil, err
		}
		snapshot := map[string]any{
			"metric_thresholds":     batch.MetricThresholds,
			"rollback_timeout_mins": batch.RollbackTimeoutMins,
		}
		metricJSON, err := encodeJSON(snapshot)
		if err != nil {
			return nil, nil, err
		}
		record := &models.CanaryDeploymentRecord{
			BatchName:      batch.Name,
			TenantScope:    scopeJSON,
			MetricSnapshot: metricJSON,
			ActionTaken:    "scheduled",
		}
		dbCanaryRecords = append(dbCanaryRecords, record)
	}

	createdPlan, err := s.plans.CreatePlan(ctx, plan, dbCanaryRecords)
	if err != nil {
		return nil, nil, err
	}

	now := s.clock()
	candidate.ApprovalStatus = models.PluginReleaseApprovalApproved
	candidate.ApprovedAt = &now
	candidate.UpdatedBy = input.Actor
	if err := s.candidates.UpdateFieldsByUUID(ctx, candidate.UUID, map[string]interface{}{
		"approval_status": candidate.ApprovalStatus,
		"approved_at":     candidate.ApprovedAt,
		"updated_by":      candidate.UpdatedBy,
		"updated_at":      now,
	}); err != nil {
		return nil, nil, err
	}

	if s.instruments != nil {
		s.instruments.PipelineDuration.Record(ctx, input.WindowEnd.Sub(input.WindowStart).Seconds())
	}
	s.hooks.PlanGenerated(ctx, createdPlan)
	return createdPlan, candidate, nil
}

// GetCandidate fetches a candidate by UUID.
func (s *Service) GetCandidate(ctx context.Context, candidateID uuid.UUID) (*models.PluginReleaseCandidate, error) {
	if candidateID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	return s.candidates.GetByUUID(ctx, candidateID)
}

// UpdateCandidate mutates the supplied candidate fields and returns the updated row.
func (s *Service) UpdateCandidate(ctx context.Context, input UpdateCandidateInput) (*models.PluginReleaseCandidate, error) {
	if input.CandidateID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	candidate, err := s.candidates.GetByUUID(ctx, input.CandidateID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCandidateNotFound
		}
		return nil, err
	}
	if candidate == nil {
		return nil, ErrCandidateNotFound
	}
	fields := map[string]interface{}{}
	if strings.TrimSpace(input.BuildArtifact) != "" {
		fields["build_artifact_uri"] = strings.TrimSpace(input.BuildArtifact)
	}
	if strings.TrimSpace(input.ReleaseNotes) != "" {
		fields["release_notes"] = strings.TrimSpace(input.ReleaseNotes)
	}
	if len(input.Labels) > 0 {
		if data, err := encodeJSON(input.Labels); err == nil {
			fields["labels"] = data
		} else {
			return nil, err
		}
	}
	if len(fields) == 0 {
		return candidate, nil
	}
	now := s.clock()
	fields["updated_at"] = now
	if strings.TrimSpace(input.Actor) != "" {
		fields["updated_by"] = strings.TrimSpace(input.Actor)
	}
	if err := s.candidates.UpdateFieldsByUUID(ctx, input.CandidateID, fields); err != nil {
		return nil, err
	}
	updated, err := s.candidates.GetByUUID(ctx, input.CandidateID)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func validateCandidateInput(input SubmitCandidateInput) error {
	switch {
	case strings.TrimSpace(input.TenantID) == "",
		strings.TrimSpace(input.PluginID) == "",
		strings.TrimSpace(input.Version) == "",
		strings.TrimSpace(input.BuildArtifact) == "",
		strings.TrimSpace(input.CommitHash) == "",
		strings.TrimSpace(input.ReleaseNotes) == "":
		return ErrInvalidInput
	default:
		return nil
	}
}

func encodeJSON(value interface{}) (datatypes.JSON, error) {
	if value == nil {
		return datatypes.JSON([]byte("null")), nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode json: %w", err)
	}
	if len(payload) == 0 {
		return datatypes.JSON([]byte("null")), nil
	}
	return datatypes.JSON(payload), nil
}
