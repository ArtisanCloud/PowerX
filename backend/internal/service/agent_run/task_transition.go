package agent_run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type taskMutation func(*TaskSnapshot, time.Time) error

// mutateTask applies the task state, run version, and matching event on one
// Redis Cluster slot. Every worker result must carry the current fence.
func (s *RedisStore) mutateTask(ctx context.Context, identity Snapshot, expectedVersion, revision uint64, taskID, eventType string, mutate taskMutation) (TaskSnapshot, Snapshot, error) {
	if s == nil || s.client == nil || !validScope(identity) || expectedVersion == 0 ||
		!validTaskRevision(revision, taskID) || !strings.HasPrefix(eventType, "agent_run.") || mutate == nil {
		return TaskSnapshot{}, Snapshot{}, ErrInvalid
	}
	stateKey, eventsKey := runKeys(identity)
	_, taskKey, _ := schedulingKeys(identity)
	field := taskField(revision, taskID)
	for attempt := 0; attempt < 5; attempt++ {
		var updated Snapshot
		var task TaskSnapshot
		err := s.client.Watch(ctx, func(tx *redis.Tx) error {
			values, err := tx.HGetAll(ctx, stateKey).Result()
			if err != nil {
				return err
			}
			if len(values) == 0 {
				return ErrNotFound
			}
			current, err := decodeSnapshot(values)
			if err != nil {
				return err
			}
			if current.Version != expectedVersion || current.PlanRevision != revision || terminal(current.Status) {
				return ErrConflict
			}
			raw, err := tx.HGet(ctx, taskKey, field).Bytes()
			if errors.Is(err, redis.Nil) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
			if err := json.Unmarshal(raw, &task); err != nil {
				return err
			}
			now := s.clock().UTC()
			if err := mutate(&task, now); err != nil {
				return err
			}
			taskRaw, err := json.Marshal(task)
			if err != nil {
				return err
			}
			updated = current
			updated.Version++
			updated.EventSeq++
			updated.UpdatedAt = now
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.HSet(ctx, taskKey, field, taskRaw)
				pipe.HSet(ctx, stateKey, encodeSnapshot(updated))
				pipe.XAdd(ctx, &redis.XAddArgs{Stream: eventsKey, ID: fmt.Sprintf("%d-0", updated.EventSeq),
					Values: map[string]any{
						"type": eventType, "status": task.Status, "task_id": taskID,
						"attempt": task.Attempt, "fence": task.Fence, "pool_id": task.PoolID,
						"queue_wait_ms": task.QueueWaitMS, "at": now.Format(time.RFC3339Nano),
						"reason_code": task.ReasonCode,
					}})
				return nil
			})
			return err
		}, stateKey, taskKey)
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		return task, updated, err
	}
	return TaskSnapshot{}, Snapshot{}, ErrConflict
}

// LeaseTask is called after TaskBus delivery. A reclaimed worker must have a
// higher fencing token and the previous task lease must have expired.
func (s *RedisStore) LeaseTask(ctx context.Context, identity Snapshot, expectedVersion, revision uint64, taskID, owner string, fence uint64, until time.Time) (TaskSnapshot, Snapshot, error) {
	if strings.TrimSpace(owner) == "" || fence == 0 || !until.After(s.clock()) {
		return TaskSnapshot{}, Snapshot{}, ErrInvalid
	}
	return s.mutateTask(ctx, identity, expectedVersion, revision, taskID, "agent_run.task_status", func(task *TaskSnapshot, now time.Time) error {
		switch task.Status {
		case "queued":
		case "leased", "running", "verifying":
			if now.Before(task.LeaseUntil) {
				return ErrConflict
			}
		default:
			return ErrConflict
		}
		if fence <= task.Fence {
			return ErrConflict
		}
		task.Status, task.LeaseOwner, task.Fence, task.LeaseUntil = "leased", owner, fence, until.UTC()
		task.LeasedAt = now
		if !task.QueuedAt.IsZero() {
			task.QueueWaitMS = now.Sub(task.QueuedAt).Milliseconds()
			if task.QueueWaitMS < 0 {
				task.QueueWaitMS = 0
			}
		}
		return nil
	})
}

func (s *RedisStore) StartTask(ctx context.Context, identity Snapshot, expectedVersion, revision uint64, taskID, owner string, fence uint64) (TaskSnapshot, Snapshot, error) {
	return s.mutateTask(ctx, identity, expectedVersion, revision, taskID, "agent_run.task_started", func(task *TaskSnapshot, now time.Time) error {
		if task.Status != "leased" || task.LeaseOwner != owner || task.Fence != fence || !now.Before(task.LeaseUntil) {
			return ErrConflict
		}
		task.Status, task.StartedAt = "running", now
		return nil
	})
}

// CompleteTask requires both a result and verification evidence reference.
// The caller must persist those objects before this state transition.
func (s *RedisStore) CompleteTask(ctx context.Context, identity Snapshot, expectedVersion, revision uint64, taskID, owner string, fence uint64, resultRef, evidenceRef string) (TaskSnapshot, Snapshot, error) {
	if strings.TrimSpace(resultRef) == "" || strings.TrimSpace(evidenceRef) == "" {
		return TaskSnapshot{}, Snapshot{}, ErrInvalid
	}
	return s.mutateTask(ctx, identity, expectedVersion, revision, taskID, "agent_run.task_completed", func(task *TaskSnapshot, now time.Time) error {
		if task.Status != "running" && task.Status != "verifying" {
			return ErrConflict
		}
		if task.LeaseOwner != owner || task.Fence != fence || !now.Before(task.LeaseUntil) {
			return ErrConflict
		}
		task.Status, task.CompletedAt = "completed", now
		task.ResultRef, task.EvidenceRef = resultRef, evidenceRef
		return nil
	})
}

// FailTask records an explicit failure under the current fence. The executor
// classifies side-effect uncertainty as manual_review_required; this method
// never infers retry safety from an arbitrary Go error.
func (s *RedisStore) FailTask(ctx context.Context, identity Snapshot, expectedVersion, revision uint64, taskID, owner string, fence uint64, reasonCode string) (TaskSnapshot, Snapshot, error) {
	if strings.TrimSpace(reasonCode) == "" || len(reasonCode) > 128 {
		return TaskSnapshot{}, Snapshot{}, ErrInvalid
	}
	return s.mutateTask(ctx, identity, expectedVersion, revision, taskID, "agent_run.task_failed", func(task *TaskSnapshot, now time.Time) error {
		if task.Status != "running" && task.Status != "verifying" {
			return ErrConflict
		}
		if task.LeaseOwner != owner || task.Fence != fence || !now.Before(task.LeaseUntil) {
			return ErrConflict
		}
		task.Status, task.CompletedAt, task.ReasonCode = "failed", now, reasonCode
		return nil
	})
}

// SkipTask records a dependency failure without giving the task to a Worker.
// It is safe to repeat reconciliation because only pending tasks may be skipped.
func (s *RedisStore) SkipTask(ctx context.Context, identity Snapshot, expectedVersion, revision uint64, taskID, reasonCode string) (TaskSnapshot, Snapshot, error) {
	if reasonCode == "" || len(reasonCode) > 128 {
		return TaskSnapshot{}, Snapshot{}, ErrInvalid
	}
	return s.mutateTask(ctx, identity, expectedVersion, revision, taskID, "agent_run.task_skipped", func(task *TaskSnapshot, now time.Time) error {
		if task.Status != "pending_dependency" {
			return ErrConflict
		}
		task.Status, task.CompletedAt, task.ReasonCode = "skipped", now, reasonCode
		return nil
	})
}

// ExpireQueuedTask is the queue-wait budget boundary. A worker racing this
// transition can start only if it wins the Run version CAS first.
func (s *RedisStore) ExpireQueuedTask(ctx context.Context, identity Snapshot, expectedVersion, revision uint64, taskID string) (TaskSnapshot, Snapshot, error) {
	return s.mutateTask(ctx, identity, expectedVersion, revision, taskID, "agent_run.task_failed", func(task *TaskSnapshot, now time.Time) error {
		if task.Status != "queued" || task.QueuedAt.IsZero() || now.Before(task.QueuedAt.Add(s.queueWaitTimeout)) {
			return ErrConflict
		}
		task.Status, task.CompletedAt, task.ReasonCode = "failed", now, "queue.timeout"
		task.QueueWaitMS = now.Sub(task.QueuedAt).Milliseconds()
		return nil
	})
}

// ExpireUnstartedTask prevents new work after the accepted Run deadline.
func (s *RedisStore) ExpireUnstartedTask(ctx context.Context, identity Snapshot, expectedVersion, revision uint64, taskID string) (TaskSnapshot, Snapshot, error) {
	return s.mutateTask(ctx, identity, expectedVersion, revision, taskID, "agent_run.task_failed", func(task *TaskSnapshot, now time.Time) error {
		switch task.Status {
		case "queued", "retry_wait":
			task.Status = "failed"
		case "pending_dependency":
			task.Status = "skipped"
		default:
			return ErrConflict
		}
		task.CompletedAt, task.ReasonCode = now, "run.deadline"
		if !task.QueuedAt.IsZero() {
			task.QueueWaitMS = now.Sub(task.QueuedAt).Milliseconds()
		}
		return nil
	})
}

// RenewTaskLease persists the heartbeat in Redis without generating a user
// event or a business DB write. A stale worker cannot extend a new fence.
func (s *RedisStore) RenewTaskLease(ctx context.Context, identity Snapshot, revision uint64, taskID, owner string, fence uint64, until time.Time) (TaskSnapshot, error) {
	if s == nil || s.client == nil || !validScope(identity) || !validTaskRevision(revision, taskID) || strings.TrimSpace(owner) == "" ||
		fence == 0 || !until.After(s.clock()) {
		return TaskSnapshot{}, ErrInvalid
	}
	_, taskKey, _ := schedulingKeys(identity)
	field := taskField(revision, taskID)
	for attempt := 0; attempt < 5; attempt++ {
		var task TaskSnapshot
		err := s.client.Watch(ctx, func(tx *redis.Tx) error {
			raw, err := tx.HGet(ctx, taskKey, field).Bytes()
			if errors.Is(err, redis.Nil) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
			if err := json.Unmarshal(raw, &task); err != nil {
				return err
			}
			if task.Status != "leased" && task.Status != "running" && task.Status != "verifying" {
				return ErrConflict
			}
			if task.LeaseOwner != owner || task.Fence != fence ||
				!s.clock().Before(task.LeaseUntil) || !until.After(task.LeaseUntil) {
				return ErrConflict
			}
			task.LeaseUntil = until.UTC()
			encoded, err := json.Marshal(task)
			if err != nil {
				return err
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.HSet(ctx, taskKey, field, encoded)
				return nil
			})
			return err
		}, taskKey)
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		return task, err
	}
	return TaskSnapshot{}, ErrConflict
}
