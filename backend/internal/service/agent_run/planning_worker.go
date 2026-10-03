package agent_run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

// PlanningResult references a persisted, verified plan artifact. Build must
// have no business side effects; only CompletePlanning publishes the DAG.
type PlanningResult struct {
	ReasonCode  string
	Plan        Plan
	ResultRef   string
	EvidenceRef string
}

// PlanBuilder derives one immutable DAG from the persisted invocation input.
type PlanBuilder interface {
	Build(context.Context, TaskRef, string) (PlanningResult, error)
}

// ProcessPlanningDelivery handles the special revision-zero planning task.
// The same fencing and heartbeat rules as regular tasks protect its commit.
func (s *RedisStore) ProcessPlanningDelivery(ctx context.Context, queue LeasedTaskQueue,
	delivery event_bus.StreamTaskDelivery, builder PlanBuilder) error {
	if s == nil || queue == nil || builder == nil {
		return ErrInvalid
	}
	var ref TaskRef
	if err := json.Unmarshal(delivery.Message.Payload, &ref); err != nil {
		return err
	}
	identity := Snapshot{TenantUUID: ref.TenantUUID, Env: ref.Env, RunID: ref.RunID}
	expectedID := fmt.Sprintf("%s:0:%s:1", ref.RunID, PlanningTaskID)
	if !validScope(identity) || ref.Revision != 0 || ref.TaskID != PlanningTaskID || ref.Attempt != 1 ||
		delivery.Message.ID != expectedID || delivery.Lease.MessageID != expectedID ||
		delivery.Message.TenantKey != taskQueueTenant(identity) || delivery.Message.SubscriberID != AgentTaskSubscriber {
		return ErrInvalid
	}
	run, err := s.Get(ctx, identity.TenantUUID, identity.Env, identity.RunID)
	if err != nil {
		return err
	}
	if run.PlanRevision > 0 || terminal(run.Status) {
		if err := queue.Ack(ctx, delivery); err != nil {
			return err
		}
		if !terminal(run.Status) {
			_, err = s.RecoverRun(ctx, identity, queue)
		}
		return err
	}
	if run.Status != "planning" || !run.DeadlineAt.After(s.clock()) {
		return ErrConflict
	}
	task, err := s.GetTask(ctx, identity, 0, PlanningTaskID)
	if err != nil || task.Attempt != ref.Attempt {
		return ErrConflict
	}
	_, run, err = s.LeaseTask(ctx, identity, run.Version, 0, PlanningTaskID,
		delivery.Lease.Owner, delivery.Lease.Token, delivery.Lease.ExpiresAt)
	if err != nil {
		return err
	}
	_, run, err = s.StartTask(ctx, identity, run.Version, 0, PlanningTaskID,
		delivery.Lease.Owner, delivery.Lease.Token)
	if err != nil {
		return err
	}
	execCtx, cancel := context.WithDeadline(ctx, run.DeadlineAt)
	defer cancel()
	interval := time.Until(delivery.Lease.ExpiresAt) / 3
	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}
	if interval > 10*time.Second {
		interval = 10 * time.Second
	}
	stop := make(chan struct{})
	heartbeatDone := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		current := delivery
		for {
			select {
			case <-stop:
				heartbeatDone <- nil
				return
			case <-execCtx.Done():
				heartbeatDone <- execCtx.Err()
				return
			case <-ticker.C:
				next, err := queue.Renew(execCtx, current)
				if err == nil {
					_, err = s.RenewTaskLease(execCtx, identity, 0, PlanningTaskID,
						next.Lease.Owner, next.Lease.Token, next.Lease.ExpiresAt)
				}
				if err != nil {
					cancel()
					heartbeatDone <- err
					return
				}
				current = next
			}
		}
	}()
	key := fmt.Sprintf("agent:%s:plan:1", ref.RunID)
	result, buildErr := buildPlanningSafely(execCtx, builder, ref, key)
	close(stop)
	heartbeatErr := <-heartbeatDone
	if heartbeatErr != nil && !errors.Is(heartbeatErr, context.Canceled) && !errors.Is(heartbeatErr, context.DeadlineExceeded) {
		return heartbeatErr
	}
	if execCtx.Err() != nil {
		return execCtx.Err()
	}
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finishCancel()
	if buildErr != nil {
		reason := result.ReasonCode
		if reason == "" {
			reason = "planner.failed"
		}
		for attempt := 0; attempt < 5; attempt++ {
			current, err := s.Get(finishCtx, identity.TenantUUID, identity.Env, identity.RunID)
			if err != nil {
				return err
			}
			if terminal(current.Status) {
				return queue.Ack(finishCtx, delivery)
			}
			_, _, err = s.FailTask(finishCtx, identity, current.Version, 0, PlanningTaskID, delivery.Lease.Owner, delivery.Lease.Token, reason)
			if errors.Is(err, ErrConflict) {
				continue
			}
			if err != nil {
				return err
			}
			if _, err = s.ExpirePlanning(finishCtx, identity); err != nil {
				return err
			}
			return queue.Ack(finishCtx, delivery)
		}
		return ErrConflict
	}
	for attempt := 0; attempt < 5; attempt++ {
		run, err = s.Get(finishCtx, identity.TenantUUID, identity.Env, identity.RunID)
		if err != nil {
			return err
		}
		_, err = s.CompletePlanning(finishCtx, identity, run.Version, delivery.Lease.Owner,
			delivery.Lease.Token, result.Plan, result.ResultRef, result.EvidenceRef)
		if errors.Is(err, ErrConflict) && run.PlanRevision == 0 {
			continue
		}
		if err != nil {
			return err
		}
		if err := queue.Ack(finishCtx, delivery); err != nil {
			return err
		}
		_, err = s.RecoverRun(finishCtx, identity, queue)
		return err
	}
	return ErrConflict
}

func buildPlanningSafely(ctx context.Context, builder PlanBuilder, ref TaskRef, key string) (result PlanningResult, err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("planner panicked")
		}
	}()
	return builder.Build(ctx, ref, key)
}
