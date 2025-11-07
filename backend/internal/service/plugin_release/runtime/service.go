package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/instrumentation"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_release"
	"gorm.io/datatypes"
)

var (
	// ErrInvalidInput indicates missing or malformed parameters.
	ErrInvalidInput = errors.New("plugin_release.runtime: invalid input")
	// ErrPlanNotFound indicates the rollout plan cannot be located.
	ErrPlanNotFound = errors.New("plugin_release.runtime: plan not found")
	// ErrBatchNotFound indicates the requested batch does not exist on the plan.
	ErrBatchNotFound = errors.New("plugin_release.runtime: batch not found")
)

// Options configures runtime behaviour.
type Options struct {
	RollbackTimeout time.Duration
}

// Dependencies enumerates required collaborators.
type Dependencies struct {
	Plans       *repo.ReleasePlanRepository
	Candidates  *repo.ReleaseCandidateRepository
	Instruments *instrumentation.Instruments
	Hooks       EventHooks
	Clock       func() time.Time
}

// Service orchestrates runtime canary executions and rollbacks.
type Service struct {
	plans       *repo.ReleasePlanRepository
	candidates  *repo.ReleaseCandidateRepository
	instruments *instrumentation.Instruments
	hooks       EventHooks
	clock       func() time.Time
	opts        Options
}

// TriggerCanaryInput describes a deployment trigger request.
type TriggerCanaryInput struct {
	PlanID    uint64
	BatchName string
	Actor     string
}

// ProgressEvent captures observable information streamed to clients.
type ProgressEvent struct {
	PlanID            uint64  `json:"planId"`
	BatchName         string  `json:"batchName"`
	Phase             string  `json:"phase"`
	ErrorRate         float64 `json:"errorRate"`
	LatencyP95MS      float64 `json:"latencyP95Ms"`
	ThresholdBreached bool    `json:"thresholdBreached"`
	Message           string  `json:"message,omitempty"`
}

// FinalizeInput describes rollout finalization.
type FinalizeInput struct {
	PlanID uint64
	Action string
	Actor  string
}

// NewService constructs the runtime service.
func NewService(deps Dependencies, opts Options) *Service {
	if deps.Plans == nil || deps.Candidates == nil {
		panic("runtime service requires non-nil repositories")
	}
	if deps.Clock == nil {
		deps.Clock = time.Now
	}
	if opts.RollbackTimeout <= 0 {
		opts.RollbackTimeout = 5 * time.Minute
	}
	if deps.Hooks == nil {
		deps.Hooks = noopHooks{}
	}
	return &Service{
		plans:       deps.Plans,
		candidates:  deps.Candidates,
		instruments: deps.Instruments,
		hooks:       deps.Hooks,
		clock:       deps.Clock,
		opts:        opts,
	}
}

// TriggerCanary executes the requested batch and emits synthetic progress events.
func (s *Service) TriggerCanary(ctx context.Context, input TriggerCanaryInput) ([]ProgressEvent, error) {
	if input.PlanID == 0 || strings.TrimSpace(input.BatchName) == "" {
		return nil, ErrInvalidInput
	}
	plan, err := s.plans.GetPlanByID(ctx, input.PlanID)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, ErrPlanNotFound
	}

	record := findCanaryRecord(plan.CanaryRecords, input.BatchName)
	if record == nil {
		return nil, ErrBatchNotFound
	}

	metadata := parseMetricMetadata(record.MetricSnapshot)
	if err := s.plans.UpdatePlanStatus(ctx, plan.ID, models.ReleasePlanStatusExecuting, nil, nil); err != nil {
		return nil, err
	}

	payload := map[string]any{
		"plan_id":    plan.ID,
		"batch_name": input.BatchName,
		"actor":      input.Actor,
	}
	s.hooks.OnCanaryStarted(ctx, payload)

	now := s.now()
	events := []ProgressEvent{{
		PlanID:    plan.ID,
		BatchName: input.BatchName,
		Phase:     "starting",
	}}
	s.recordPhaseMetric(ctx, "starting", 0)

	observed := s.observeMetrics(metadata)
	monitorStart := s.now()
	events = append(events, ProgressEvent{
		PlanID:       plan.ID,
		BatchName:    input.BatchName,
		Phase:        "monitoring",
		ErrorRate:    observed.ErrorRate,
		LatencyP95MS: observed.LatencyP95MS,
	})
	s.recordPhaseMetric(ctx, "monitoring", s.now().Sub(monitorStart))
	s.recordErrorRate(ctx, observed.ErrorRate)

	threshold := metadata.MetricThresholds["error_rate"]
	thresholdBreached := threshold > 0 && observed.ErrorRate > threshold
	record.ThresholdBreached = thresholdBreached
	record.CompletedAt = toTimePtr(now)
	payload["error_rate"] = observed.ErrorRate
	payload["threshold"] = threshold
	if thresholdBreached {
		record.ActionTaken = models.CanaryActionRollback
		s.recordRollback(ctx, now.Sub(plan.WindowStart))
		if err := s.plans.UpdatePlanStatus(ctx, plan.ID, models.ReleasePlanStatusRolledBack, nil, nil); err != nil {
			return nil, err
		}
	} else {
		record.ActionTaken = models.CanaryActionContinue
		s.hooks.OnCanaryProgress(ctx, payload)
	}
	if _, err := s.plans.AppendCanarySnapshot(ctx, record); err != nil {
		return nil, err
	}
	finalPhase := "completed"
	if thresholdBreached {
		finalPhase = "rolled_back"
	}
	events = append(events, ProgressEvent{
		PlanID:            plan.ID,
		BatchName:         input.BatchName,
		Phase:             finalPhase,
		ErrorRate:         observed.ErrorRate,
		LatencyP95MS:      observed.LatencyP95MS,
		ThresholdBreached: thresholdBreached,
	})
	if thresholdBreached {
		s.hooks.OnCanaryRolledBack(ctx, payload)
	} else {
		s.hooks.OnCanaryCompleted(ctx, payload)
	}
	return events, nil
}

// FinalizeDeployment marks the plan according to action (promote|rollback).
func (s *Service) FinalizeDeployment(ctx context.Context, input FinalizeInput) (*models.ReleasePlan, error) {
	if input.PlanID == 0 {
		return nil, ErrInvalidInput
	}
	plan, err := s.plans.GetPlanByID(ctx, input.PlanID)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, ErrPlanNotFound
	}

	action := strings.ToLower(strings.TrimSpace(input.Action))
	var targetStatus string
	switch action {
	case "promote", "complete", "completed":
		targetStatus = models.ReleasePlanStatusCompleted
	case "rollback", "rolled_back":
		targetStatus = models.ReleasePlanStatusRolledBack
	default:
		return nil, ErrInvalidInput
	}
	if err := s.plans.UpdatePlanStatus(ctx, plan.ID, targetStatus, nil, nil); err != nil {
		return nil, err
	}
	payload := map[string]any{
		"plan_id": plan.ID,
		"actor":   input.Actor,
		"action":  targetStatus,
	}
	if targetStatus == models.ReleasePlanStatusCompleted {
		s.hooks.OnCanaryCompleted(ctx, payload)
	} else {
		s.hooks.OnCanaryRolledBack(ctx, payload)
	}
	plan.Status = targetStatus
	return plan, nil
}

type metricMetadata struct {
	MetricThresholds    map[string]float64 `json:"metric_thresholds"`
	RollbackTimeoutMins uint32             `json:"rollback_timeout_minutes"`
}

type observedMetrics struct {
	ErrorRate    float64
	LatencyP95MS float64
}

func (s *Service) observeMetrics(meta metricMetadata) observedMetrics {
	const defaultErrorRate = 0.02
	const defaultLatency = 250.0
	return observedMetrics{
		ErrorRate:    defaultErrorRate,
		LatencyP95MS: defaultLatency,
	}
}

func (s *Service) recordPhaseMetric(ctx context.Context, phase string, duration time.Duration) {
	if s.instruments == nil {
		return
	}
	s.instruments.RecordCanaryPhase(ctx, phase, duration)
}

func (s *Service) recordErrorRate(ctx context.Context, value float64) {
	if s.instruments == nil {
		return
	}
	s.instruments.RecordCanaryErrorRate(ctx, value)
}

func (s *Service) recordRollback(ctx context.Context, duration time.Duration) {
	if s.instruments == nil {
		return
	}
	s.instruments.CanaryRollbackLatency.Record(ctx, duration.Seconds())
	s.instruments.IncrementCanaryRollback(ctx)
}

func parseMetricMetadata(raw datatypes.JSON) metricMetadata {
	if len(raw) == 0 {
		return metricMetadata{MetricThresholds: map[string]float64{}}
	}
	var meta metricMetadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		return metricMetadata{MetricThresholds: map[string]float64{}}
	}
	if meta.MetricThresholds == nil {
		meta.MetricThresholds = map[string]float64{}
	}
	return meta
}

func findCanaryRecord(records []models.CanaryDeploymentRecord, batchName string) *models.CanaryDeploymentRecord {
	for i := range records {
		if strings.EqualFold(records[i].BatchName, batchName) {
			return &records[i]
		}
	}
	return nil
}

func toTimePtr(t time.Time) *time.Time {
	return &t
}

func (s *Service) now() time.Time {
	if s.clock != nil {
		return s.clock()
	}
	return time.Now()
}
