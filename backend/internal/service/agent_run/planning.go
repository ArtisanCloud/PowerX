package agent_run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const PlanningTaskID = "plan"

// StartPlanning commits one planning request to the Run's atomic outbox.
// A retry sees the already committed request and never creates a second one.
func (s *RedisStore) StartPlanning(ctx context.Context, identity Snapshot) (Snapshot, error) {
	if s == nil || s.client == nil || !validScope(identity) {
		return Snapshot{}, ErrInvalid
	}
	stateKey, eventsKey := runKeys(identity)
	_, taskKey, outboxKey := schedulingKeys(identity)
	for attempt := 0; attempt < 5; attempt++ {
		var updated Snapshot
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
			if current.PlanRevision != 0 || terminal(current.Status) {
				return ErrConflict
			}
			if current.Status == "planning" {
				count, err := tx.XLen(ctx, outboxKey).Result()
				if err != nil || count == 0 {
					if err != nil {
						return err
					}
					return ErrConflict
				}
				exists, err := tx.HExists(ctx, taskKey, taskField(0, PlanningTaskID)).Result()
				if err != nil || !exists {
					if err != nil {
						return err
					}
					return ErrConflict
				}
				updated = current
				return nil
			}
			if current.Status != "accepted" || !current.DeadlineAt.After(s.clock()) {
				return ErrConflict
			}
			updated = current
			updated.Status = "planning"
			updated.Version++
			updated.EventSeq++
			updated.UpdatedAt = s.clock().UTC()
			planningTask, err := json.Marshal(TaskSnapshot{TaskID: PlanningTaskID, Revision: 0, Status: "queued",
				Attempt: 1, QueuedAt: updated.UpdatedAt})
			if err != nil {
				return err
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.HSet(ctx, stateKey, encodeSnapshot(updated))
				pipe.HSet(ctx, taskKey, taskField(0, PlanningTaskID), planningTask)
				pipe.XAdd(ctx, &redis.XAddArgs{Stream: eventsKey, ID: fmt.Sprintf("%d-0", updated.EventSeq),
					Values: map[string]any{"type": "agent_run.planning_queued", "status": updated.Status, "at": updated.UpdatedAt.Format(time.RFC3339Nano)}})
				pipe.XAdd(ctx, &redis.XAddArgs{Stream: outboxKey, Values: map[string]any{
					"run_id": identity.RunID, "revision": 0, "task_id": PlanningTaskID,
					"attempt": 1, "event_seq": updated.EventSeq,
				}})
				return nil
			})
			return err
		}, stateKey, taskKey)
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		return updated, err
	}
	return Snapshot{}, ErrConflict
}

// CompletePlanning atomically fences the planner attempt, records its
// evidence, and installs revision one. A crash after this commit can be
// recovered by ReconcilePlan without rerunning the planner.
func (s *RedisStore) CompletePlanning(ctx context.Context, identity Snapshot, expectedVersion uint64,
	owner string, fence uint64, plan Plan, resultRef, evidenceRef string) (Snapshot, error) {
	if s == nil || s.client == nil || !validScope(identity) || expectedVersion == 0 ||
		strings.TrimSpace(owner) == "" || fence == 0 || plan.Revision != 1 || !validPlan(plan) ||
		strings.TrimSpace(resultRef) == "" || strings.TrimSpace(evidenceRef) == "" {
		return Snapshot{}, ErrInvalid
	}
	stateKey, eventsKey := runKeys(identity)
	planKey, taskKey, _ := schedulingKeys(identity)
	rawPlan, err := json.Marshal(plan)
	if err != nil {
		return Snapshot{}, err
	}
	for attempt := 0; attempt < 5; attempt++ {
		var updated Snapshot
		err = s.client.Watch(ctx, func(tx *redis.Tx) error {
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
			if current.Version != expectedVersion || current.PlanRevision != 0 || current.Status != "planning" {
				return ErrConflict
			}
			rawTask, err := tx.HGet(ctx, taskKey, taskField(0, PlanningTaskID)).Bytes()
			if errors.Is(err, redis.Nil) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
			var task TaskSnapshot
			if err := json.Unmarshal(rawTask, &task); err != nil {
				return err
			}
			now := s.clock().UTC()
			if task.Status != "running" || task.LeaseOwner != owner || task.Fence != fence ||
				!now.Before(task.LeaseUntil) || !now.Before(current.DeadlineAt) {
				return ErrConflict
			}
			exists, err := tx.HExists(ctx, planKey, strconv.FormatUint(plan.Revision, 10)).Result()
			if err != nil || exists {
				if err != nil {
					return err
				}
				return ErrConflict
			}
			task.Status, task.CompletedAt = "completed", now
			task.ResultRef, task.EvidenceRef = resultRef, evidenceRef
			completedTask, err := json.Marshal(task)
			if err != nil {
				return err
			}
			updated = current
			updated.PlanRevision = plan.Revision
			updated.Version++
			updated.EventSeq++
			updated.UpdatedAt = now
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.HSet(ctx, planKey, strconv.FormatUint(plan.Revision, 10), rawPlan)
				pipe.HSet(ctx, taskKey, taskField(0, PlanningTaskID), completedTask)
				for _, definition := range plan.Tasks {
					initial := TaskSnapshot{TaskID: definition.TaskID, Revision: plan.Revision,
						Status: "pending_dependency", DependsOn: definition.DependsOn, PoolID: definition.PoolID}
					raw, marshalErr := json.Marshal(initial)
					if marshalErr != nil {
						return marshalErr
					}
					pipe.HSet(ctx, taskKey, taskField(plan.Revision, definition.TaskID), raw)
				}
				pipe.HSet(ctx, stateKey, encodeSnapshot(updated))
				pipe.XAdd(ctx, &redis.XAddArgs{Stream: eventsKey, ID: fmt.Sprintf("%d-0", updated.EventSeq),
					Values: map[string]any{"type": "agent_run.plan_created", "status": updated.Status,
						"task_id": PlanningTaskID, "attempt": task.Attempt, "at": now.Format(time.RFC3339Nano)}})
				return nil
			})
			return err
		}, stateKey, planKey, taskKey)
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		return updated, err
	}
	return Snapshot{}, ErrConflict
}

// ExpirePlanning closes a planner that never acquired capacity or exceeded
// the Run deadline. The task receipt and terminal Run event commit together.
func (s *RedisStore) ExpirePlanning(ctx context.Context, identity Snapshot) (Snapshot, error) {
	if s == nil || s.client == nil || !validScope(identity) {
		return Snapshot{}, ErrInvalid
	}
	stateKey, eventsKey := runKeys(identity)
	_, taskKey, _ := schedulingKeys(identity)
	for attempt := 0; attempt < 5; attempt++ {
		var updated Snapshot
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
			updated = current
			now := s.clock().UTC()
			if current.PlanRevision == 0 && current.Status == "accepted" && !now.Before(current.DeadlineAt) {
				updated.Status = "failed"
				updated.Version++
				updated.EventSeq++
				updated.UpdatedAt = now
				_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
					pipe.HSet(ctx, stateKey, encodeSnapshot(updated))
					pipe.XAdd(ctx, &redis.XAddArgs{Stream: eventsKey, ID: fmt.Sprintf("%d-0", updated.EventSeq),
						Values: map[string]any{"type": "agent_run.final", "status": updated.Status,
							"reason_code": "run.deadline", "at": now.Format(time.RFC3339Nano)}})
					return nil
				})
				return err
			}
			if current.PlanRevision != 0 || current.Status != "planning" {
				return nil
			}
			raw, err := tx.HGet(ctx, taskKey, taskField(0, PlanningTaskID)).Bytes()
			if err != nil {
				return err
			}
			var task TaskSnapshot
			if err := json.Unmarshal(raw, &task); err != nil {
				return err
			}
			reason := ""
			switch {
			case task.Status == "failed":
				reason = task.ReasonCode
				if reason == "" {
					reason = "planner.failed"
				}
			case !now.Before(current.DeadlineAt):
				reason = "run.deadline"
			case task.Status == "queued" && !now.Before(task.QueuedAt.Add(s.queueWaitTimeout)):
				reason = "queue.timeout"
			}
			if reason == "" {
				return nil
			}
			task.Status, task.CompletedAt, task.ReasonCode = "failed", now, reason
			if !task.QueuedAt.IsZero() {
				task.QueueWaitMS = now.Sub(task.QueuedAt).Milliseconds()
			}
			taskRaw, err := json.Marshal(task)
			if err != nil {
				return err
			}
			updated.Status = "failed"
			updated.Version++
			updated.EventSeq++
			updated.UpdatedAt = now
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.HSet(ctx, taskKey, taskField(0, PlanningTaskID), taskRaw)
				pipe.HSet(ctx, stateKey, encodeSnapshot(updated))
				pipe.XAdd(ctx, &redis.XAddArgs{Stream: eventsKey, ID: fmt.Sprintf("%d-0", updated.EventSeq),
					Values: map[string]any{"type": "agent_run.final", "status": updated.Status,
						"task_id": PlanningTaskID, "attempt": task.Attempt, "reason_code": reason,
						"at": now.Format(time.RFC3339Nano)}})
				return nil
			})
			return err
		}, stateKey, taskKey)
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		return updated, err
	}
	return Snapshot{}, ErrConflict
}
