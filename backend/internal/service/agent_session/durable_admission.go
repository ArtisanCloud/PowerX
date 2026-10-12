package agent_session

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/agent"
	"gorm.io/gorm"
)

var runEnvironment = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,32}$`)

// DurableRunStore binds the low-frequency SQL admission anchor to the
// authoritative Redis Run without placing execution events in SQL.
type DurableRunStore interface {
	RunEventStore
	Create(context.Context, agent_run.Snapshot) (agent_run.Snapshot, error)
	RecoverAdmission(context.Context, agent_run.Snapshot) (agent_run.Snapshot, error)
	StartPlanning(context.Context, agent_run.Snapshot) (agent_run.Snapshot, error)
	DispatchPending(context.Context, agent_run.Snapshot, agent_run.TaskEnqueuer, int64) (int, error)
}

// ConfigureDurableAdmission installs the durable mode only on a service with
// no process-local executor. Worker planning and finalization must be wired
// before this service is put into the formal production Deps.
func (s *Service) ConfigureDurableAdmission(store DurableRunStore, queue agent_run.TaskEnqueuer, env string, deadline time.Duration) error {
	if s == nil || store == nil || queue == nil || s.executor != nil || !runEnvironment.MatchString(env) ||
		deadline < time.Minute || deadline > 24*time.Hour || s.runEvents != nil {
		return ErrInvalid
	}
	s.durableRuns, s.runEvents, s.runEnv, s.runDeadline, s.planningQueue = store, store, env, deadline, queue
	return nil
}

// submitPlanning commits the one planning outbox entry, then best-effort
// dispatches it. A retry or recovery scanner can dispatch it after a crash.
func (s *Service) submitPlanning(ctx context.Context, owner repo.SessionOwner, run *m.ServiceInvocation) error {
	if run.FinishedAt != nil {
		return nil
	}
	identity := durableIdentity(owner, run)
	_, err := s.durableRuns.StartPlanning(ctx, identity)
	if errors.Is(err, agent_run.ErrConflict) {
		current, readErr := s.durableRuns.Get(ctx, identity.TenantUUID, identity.Env, identity.RunID)
		if readErr == nil && (current.PlanRevision > 0 || durableTerminalStatus(current.Status)) {
			return nil
		}
	}
	if err != nil {
		return ErrDependency
	}
	if _, err := s.durableRuns.DispatchPending(ctx, identity, s.planningQueue, 100); err != nil {
		return ErrDependency
	}
	return nil
}

func durableIdentity(owner repo.SessionOwner, run *m.ServiceInvocation) agent_run.Snapshot {
	return agent_run.Snapshot{TenantUUID: owner.TenantUUID.String(), Env: run.RunEnv, RunID: run.UUID.String(),
		SessionID: run.SessionUUID.String(), MessageID: run.MessageUUID.String(), TraceID: run.TraceUUID.String(),
		Status: "accepted", DeadlineAt: run.DeadlineAt}
}

func durableTerminalStatus(status string) bool {
	switch status {
	case "completed", "partial", "needs_input", "blocked", "failed", "cancelled":
		return true
	default:
		return false
	}
}

// admitDurable never recreates a Run after SQL records admission. Missing
// Redis state at that point requires recovery from receipts, not reexecution.
func (s *Service) admitDurable(ctx context.Context, owner repo.SessionOwner, run *m.ServiceInvocation) error {
	if run.RunEnv != s.runEnv || run.AdmissionState == "" {
		return ErrConflict
	}
	identity := durableIdentity(owner, run)
	if run.AdmissionState == "admitted" {
		current, err := s.invocationSnapshot(ctx, run)
		if err != nil {
			return ErrDependency
		}
		if current.SessionID != identity.SessionID || current.MessageID != identity.MessageID ||
			current.TraceID != identity.TraceID || !current.DeadlineAt.Equal(identity.DeadlineAt) {
			return ErrDependency
		}
		return nil
	}
	if run.AdmissionState != "pending_create" || !s.now().Before(run.DeadlineAt) {
		return ErrExpired
	}
	created, err := s.durableRuns.Create(ctx, identity)
	if err != nil {
		return ErrDependency
	}
	if created.Status != "accepted" || created.PlanRevision != 0 {
		// A pending SQL anchor must never authorize replay of work already
		// started in Redis by another process.
		return ErrDependency
	}
	if err := s.repo.MarkInvocationAdmitted(ctx, owner, run.SessionUUID, run.UUID); err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrDependency
		}
		current, readErr := s.repo.Invocation(ctx, owner, run.SessionUUID, run.UUID)
		if readErr != nil || current.AdmissionState != "admitted" {
			return ErrDependency
		}
	}
	run.AdmissionState = "admitted"
	return nil
}
