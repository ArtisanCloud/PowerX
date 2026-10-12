package agent_run

import (
	"context"
	"errors"
)

// RecoverRun repairs a single admitted Run after an API/Worker process exits
// between a committed state change and outbox dispatch. The caller discovers
// active Run identities from its low-frequency admission locator and repeats
// this operation until the Run is terminal. Redis loss beyond RPO requires
// business receipt review; this method never reconstructs side effects.
func (s *RedisStore) RecoverRun(ctx context.Context, identity Snapshot, queue TaskEnqueuer) (Snapshot, error) {
	if queue == nil {
		return Snapshot{}, ErrInvalid
	}
	// The locator scanner must pass only SQL anchors marked admitted. If the
	// API died between that marker and StartPlanning, this is the missing
	// outbox transition; StartPlanning is CAS-idempotent across scanners.
	current, err := s.Get(ctx, identity.TenantUUID, identity.Env, identity.RunID)
	if err != nil {
		return Snapshot{}, err
	}
	if current.Status == "accepted" && current.PlanRevision == 0 {
		if _, err := s.StartPlanning(ctx, identity); err != nil && !errors.Is(err, ErrConflict) {
			return Snapshot{}, err
		}
	}
	if _, err := s.ExpirePlanning(ctx, identity); err != nil {
		return Snapshot{}, err
	}
	run, err := s.ReconcilePlan(ctx, identity)
	if err != nil || terminal(run.Status) {
		return run, err
	}
	if run.PlanRevision == 0 && run.Status != "planning" {
		return run, nil
	}
	_, err = s.DispatchPending(ctx, identity, queue, 1000)
	return run, err
}
