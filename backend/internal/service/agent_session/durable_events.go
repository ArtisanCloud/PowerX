package agent_session

import (
	"context"
	"errors"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	"github.com/google/uuid"
)

// RunEventStore is the authoritative state/event reader for a durable Run.
// The SQL invocation row is only an admission and ownership locator.
type RunEventStore interface {
	Get(context.Context, string, string, string) (agent_run.Snapshot, error)
	Events(context.Context, string, string, string, uint64, int64) ([]agent_run.Event, error)
	ListTasks(context.Context, agent_run.Snapshot, uint64) ([]agent_run.TaskSnapshot, error)
}

// RunStateSnapshot includes the current plan tasks for cursor recovery.
type RunStateSnapshot struct {
	agent_run.Snapshot
	Tasks []agent_run.TaskSnapshot `json:"tasks"`
}

// RunSubscription is an authorized reader for one admitted Run.
type RunSubscription struct {
	store       RunEventStore
	finalize    func(context.Context, agent_run.Snapshot) error
	tenant      string
	env         string
	runID       string
	sessionID   string
	messageID   string
	traceID     string
	archiveLoad func(context.Context) (agent_run.RunReport, error)
	archive     *agent_run.RunReport
}

// ConfigureRunEvents is used when the matching durable admission and Worker
// are installed. It must not be enabled for the legacy process-local executor.
func (s *Service) ConfigureRunEvents(store RunEventStore, env string) error {
	if s == nil || store == nil || s.executor != nil || strings.TrimSpace(env) != env || env == "" {
		return ErrInvalid
	}
	s.runEvents, s.runEnv = store, env
	return nil
}

// HasRunEvents reports whether this service uses authoritative Run events.
func (s *Service) HasRunEvents() bool { return s != nil && s.runEvents != nil }

// OpenRunSubscription checks STS, tenant, session and invocation ownership.
// An active SSE connection can then poll Redis without a SQL read each second.
func (s *Service) OpenRunSubscription(ctx context.Context, sessionID, runID uuid.UUID) (*RunSubscription, error) {
	owner, err := s.owner(ctx, SessionCapability)
	if err != nil {
		return nil, err
	}
	if s.runEvents == nil || sessionID == uuid.Nil || runID == uuid.Nil {
		return nil, ErrInvalid
	}
	if _, err = s.repo.GetSession(ctx, owner, sessionID); err != nil {
		return nil, translate(err)
	}
	anchor, err := s.repo.Invocation(ctx, owner, sessionID, runID)
	if err != nil {
		return nil, translate(err)
	}
	if s.durableRuns != nil && (anchor.AdmissionState != "admitted" || anchor.RunEnv != s.runEnv) {
		return nil, ErrDependency
	}
	var finalize func(context.Context, agent_run.Snapshot) error
	if s.finalOutput != nil {
		finalize = s.FinalizeRun
	}
	sub := &RunSubscription{store: s.runEvents, finalize: finalize, tenant: owner.TenantUUID.String(), env: s.runEnv,
		runID: runID.String(), sessionID: sessionID.String(), messageID: anchor.MessageUUID.String(), traceID: anchor.TraceUUID.String()}
	if s.archiveObjects != nil && anchor.FinishedAt != nil && anchor.ArchivedAt != nil {
		sub.archiveLoad = func(ctx context.Context) (agent_run.RunReport, error) { return s.readArchivedInvocation(ctx, anchor) }
	}
	return sub, nil
}

// RunEvents opens and reads one authorized page. Use a subscription for SSE.
func (s *Service) RunEvents(ctx context.Context, sessionID, runID uuid.UUID, afterSeq uint64, limit int64) (agent_run.Snapshot, []agent_run.Event, error) {
	subscription, err := s.OpenRunSubscription(ctx, sessionID, runID)
	if err != nil {
		return agent_run.Snapshot{}, nil, err
	}
	return subscription.Read(ctx, afterSeq, limit)
}

// Read returns one Redis page. The HTTP stream periodically reopens the
// subscription to catch grant revocation during a long-lived connection.
func (sub *RunSubscription) Read(ctx context.Context, afterSeq uint64, limit int64) (agent_run.Snapshot, []agent_run.Event, error) {
	if sub == nil || limit < 1 || limit > 1000 {
		return agent_run.Snapshot{}, nil, ErrInvalid
	}
	run, err := sub.getRun(ctx)
	if err != nil {
		return agent_run.Snapshot{}, nil, ErrDependency
	}
	if run.SessionID != sub.sessionID || run.MessageID != sub.messageID || run.TraceID != sub.traceID {
		return agent_run.Snapshot{}, nil, ErrDependency
	}
	if durableTerminalStatus(run.Status) && sub.finalize != nil && sub.archive == nil {
		if err := sub.finalize(ctx, run); err != nil {
			return agent_run.Snapshot{}, nil, ErrDependency
		}
	}
	if afterSeq > run.EventSeq {
		return run, nil, ErrEventCursorExpired
	}
	var events []agent_run.Event
	if sub.archive != nil {
		for _, event := range sub.archive.Events {
			if event.Seq > afterSeq {
				events = append(events, event)
				if int64(len(events)) == limit {
					break
				}
			}
		}
	} else {
		events, err = sub.store.Events(ctx, sub.tenant, sub.env, sub.runID, afterSeq, limit)
	}
	if errors.Is(err, agent_run.ErrEventCursorExpired) {
		return run, nil, ErrEventCursorExpired
	}
	if err != nil {
		return agent_run.Snapshot{}, nil, ErrDependency
	}
	return run, events, nil
}

// Snapshot returns a version-consistent Run and task view after event trim.
func (sub *RunSubscription) SnapshotState(ctx context.Context) (RunStateSnapshot, error) {
	if sub == nil {
		return RunStateSnapshot{}, ErrInvalid
	}
	for i := 0; i < 3; i++ {
		run, _, err := sub.Read(ctx, 0, 1)
		if err != nil && !errors.Is(err, ErrEventCursorExpired) {
			return RunStateSnapshot{}, err
		}
		var tasks []agent_run.TaskSnapshot
		if sub.archive != nil {
			for _, task := range sub.archive.Tasks {
				if task.Revision == run.PlanRevision {
					tasks = append(tasks, task)
				}
			}
		} else {
			tasks, err = sub.store.ListTasks(ctx, run, run.PlanRevision)
		}
		if err != nil {
			return RunStateSnapshot{}, ErrDependency
		}
		latest, err := sub.getRun(ctx)
		if err != nil {
			return RunStateSnapshot{}, ErrDependency
		}
		if latest.Version == run.Version {
			if durableTerminalStatus(run.Status) && sub.finalize != nil && sub.archive == nil {
				if err := sub.finalize(ctx, run); err != nil {
					return RunStateSnapshot{}, ErrDependency
				}
			}
			return RunStateSnapshot{Snapshot: run, Tasks: tasks}, nil
		}
	}
	return RunStateSnapshot{}, ErrDependency
}

func (sub *RunSubscription) getRun(ctx context.Context) (agent_run.Snapshot, error) {
	if sub.archive != nil {
		return sub.archive.Snapshot, nil
	}
	run, err := sub.store.Get(ctx, sub.tenant, sub.env, sub.runID)
	if errors.Is(err, agent_run.ErrNotFound) && sub.archiveLoad != nil {
		report, readErr := sub.archiveLoad(ctx)
		if readErr != nil {
			return agent_run.Snapshot{}, readErr
		}
		sub.archive = &report
		return report.Snapshot, nil
	}
	return run, err
}
