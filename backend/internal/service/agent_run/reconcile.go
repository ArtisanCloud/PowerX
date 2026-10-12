package agent_run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// ReconcilePlan advances one immutable plan revision. Concurrent reconcilers
// may race: QueueTask, SkipTask, and the final Run transition all use the
// observed Run version, so only one state/event/outbox write wins each step.
// The caller must also dispatch the outbox; this method never executes work.
func (s *RedisStore) ReconcilePlan(ctx context.Context, identity Snapshot) (Snapshot, error) {
	if s == nil || s.client == nil || !validScope(identity) {
		return Snapshot{}, ErrInvalid
	}
	for step := 0; step < 4096; step++ {
		run, err := s.Get(ctx, identity.TenantUUID, identity.Env, identity.RunID)
		if err != nil || terminal(run.Status) {
			return run, err
		}
		if run.PlanRevision == 0 {
			return run, nil
		}
		plan, err := s.GetPlan(ctx, identity, run.PlanRevision)
		if err != nil {
			return Snapshot{}, err
		}
		tasks, err := s.planTasks(ctx, identity, plan)
		if err != nil {
			return Snapshot{}, err
		}
		states := make(map[string]TaskSnapshot, len(tasks))
		for _, task := range tasks {
			states[task.TaskID] = task
		}
		changed := false
		if !run.DeadlineAt.After(s.clock()) {
			for _, task := range tasks {
				if task.Status != "queued" && task.Status != "retry_wait" && task.Status != "pending_dependency" {
					continue
				}
				_, _, err = s.ExpireUnstartedTask(ctx, identity, run.Version, run.PlanRevision, task.TaskID)
				if err != nil && !errors.Is(err, ErrConflict) {
					return Snapshot{}, err
				}
				changed = true
				break
			}
			if changed {
				continue
			}
		}
		for _, task := range tasks {
			if task.Status != "queued" || task.QueuedAt.IsZero() || s.clock().Before(task.QueuedAt.Add(s.queueWaitTimeout)) {
				continue
			}
			_, _, err = s.ExpireQueuedTask(ctx, identity, run.Version, run.PlanRevision, task.TaskID)
			if err != nil && !errors.Is(err, ErrConflict) {
				return Snapshot{}, err
			}
			changed = true
			break
		}
		if changed {
			continue
		}
		if run.DeadlineAt.After(s.clock()) {
			for _, task := range tasks {
				if task.Status != "pending_dependency" {
					continue
				}
				ready, blocked := true, false
				for _, depID := range task.DependsOn {
					switch states[depID].Status {
					case "completed":
					case "failed", "skipped", "cancelled":
						blocked = true
					default:
						ready = false
					}
				}
				if blocked {
					_, _, err = s.SkipTask(ctx, identity, run.Version, run.PlanRevision, task.TaskID, "dependency.failed")
				} else if ready {
					_, _, err = s.QueueTask(ctx, identity, run.Version, run.PlanRevision, task.TaskID, "dependency_ready")
				} else {
					continue
				}
				if errors.Is(err, ErrConflict) {
					changed = true
					break
				}
				if err != nil {
					return Snapshot{}, err
				}
				changed = true
				break
			}
		}
		if changed {
			continue
		}
		completed, failed, skipped, manualReview := 0, 0, 0, false
		allDone := true
		for _, task := range tasks {
			switch task.Status {
			case "completed":
				completed++
			case "failed":
				failed++
				manualReview = manualReview || task.ReasonCode == "manual_review_required"
			case "skipped", "cancelled":
				skipped++
			default:
				allDone = false
			}
		}
		if !allDone {
			return run, nil
		}
		status := "completed"
		switch {
		case manualReview:
			status = "blocked"
		case failed+skipped > 0 && completed > 0:
			status = "partial"
		case failed+skipped > 0:
			status = "failed"
		}
		updated, err := s.Transition(ctx, identity, run.Version, status, "agent_run.final")
		if errors.Is(err, ErrConflict) {
			continue
		}
		return updated, err
	}
	return Snapshot{}, ErrConflict
}

func (s *RedisStore) GetPlan(ctx context.Context, identity Snapshot, revision uint64) (Plan, error) {
	if s == nil || s.client == nil || !validScope(identity) || revision == 0 {
		return Plan{}, ErrInvalid
	}
	planKey, _, _ := schedulingKeys(identity)
	raw, err := s.client.HGet(ctx, planKey, strconv.FormatUint(revision, 10)).Bytes()
	if errors.Is(err, redis.Nil) {
		return Plan{}, ErrNotFound
	}
	if err != nil {
		return Plan{}, err
	}
	var plan Plan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return Plan{}, err
	}
	if plan.Revision != revision || !validPlan(plan) {
		return Plan{}, fmt.Errorf("%w: stored plan is invalid", ErrInvalid)
	}
	return plan, nil
}

func (s *RedisStore) planTasks(ctx context.Context, identity Snapshot, plan Plan) ([]TaskSnapshot, error) {
	_, taskKey, _ := schedulingKeys(identity)
	fields := make([]string, len(plan.Tasks))
	for i, definition := range plan.Tasks {
		fields[i] = taskField(plan.Revision, definition.TaskID)
	}
	values, err := s.client.HMGet(ctx, taskKey, fields...).Result()
	if err != nil {
		return nil, err
	}
	tasks := make([]TaskSnapshot, len(fields))
	for i, raw := range values {
		if raw == nil {
			return nil, ErrNotFound
		}
		if err := json.Unmarshal([]byte(fmt.Sprint(raw)), &tasks[i]); err != nil {
			return nil, err
		}
		if tasks[i].TaskID != plan.Tasks[i].TaskID || tasks[i].Revision != plan.Revision {
			return nil, ErrInvalid
		}
	}
	return tasks, nil
}
