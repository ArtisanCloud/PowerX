package agent_run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

var taskIDPattern = regexp.MustCompile("^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,127}$")

// Plan and TaskDefinition contain scheduling metadata only.
type Plan struct {
	Revision uint64           `json:"revision"`
	Tasks    []TaskDefinition `json:"tasks"`
}

type TaskDefinition struct {
	TaskID    string   `json:"task_id"`
	DependsOn []string `json:"depends_on"`
	PoolID    string   `json:"pool_id"`
}

type TaskSnapshot struct {
	TaskID               string    `json:"task_id"`
	Revision             uint64    `json:"revision"`
	Status               string    `json:"status"`
	DependsOn            []string  `json:"depends_on"`
	PoolID               string    `json:"pool_id"`
	Attempt              uint64    `json:"attempt"`
	QueuedAt             time.Time `json:"queued_at,omitempty"`
	QueueReason          string    `json:"queue_reason,omitempty"`
	LeasedAt             time.Time `json:"leased_at,omitempty"`
	QueueWaitMS          int64     `json:"queue_wait_ms,omitempty"`
	LeaseOwner           string    `json:"lease_owner,omitempty"`
	Fence                uint64    `json:"fence"`
	LeaseUntil           time.Time `json:"lease_until,omitempty"`
	StartedAt            time.Time `json:"started_at,omitempty"`
	CompletedAt          time.Time `json:"completed_at,omitempty"`
	ResultRef            string    `json:"result_ref,omitempty"`
	EvidenceRef          string    `json:"evidence_ref,omitempty"`
	ReasonCode           string    `json:"reason_code,omitempty"`
	ExecutionToken       string    `json:"execution_token,omitempty"`
	ExecutionResultRef   string    `json:"execution_result_ref,omitempty"`
	ExecutionEvidenceRef string    `json:"execution_evidence_ref,omitempty"`
}

// InstallPlan writes an immutable revision, initial task states, Run state,
// and the matching event in one Redis transaction on the same hash slot.
func (s *RedisStore) InstallPlan(ctx context.Context, identity Snapshot, expectedVersion uint64, plan Plan) (Snapshot, error) {
	if s == nil || s.client == nil || !validScope(identity) || expectedVersion == 0 || !validPlan(plan) {
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
			if current.Version != expectedVersion || terminal(current.Status) || plan.Revision != current.PlanRevision+1 {
				return ErrConflict
			}
			exists, err := tx.HExists(ctx, planKey, strconv.FormatUint(plan.Revision, 10)).Result()
			if err != nil || exists {
				if err != nil {
					return err
				}
				return ErrConflict
			}
			updated = current
			updated.PlanRevision = plan.Revision
			updated.Status = "planning"
			updated.Version++
			updated.EventSeq++
			updated.UpdatedAt = s.clock().UTC()
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.HSet(ctx, planKey, strconv.FormatUint(plan.Revision, 10), rawPlan)
				for _, definition := range plan.Tasks {
					task := TaskSnapshot{
						TaskID: definition.TaskID, Revision: plan.Revision,
						Status: "pending_dependency", DependsOn: definition.DependsOn,
						PoolID: definition.PoolID,
					}
					raw, marshalErr := json.Marshal(task)
					if marshalErr != nil {
						return marshalErr
					}
					pipe.HSet(ctx, taskKey, taskField(task.Revision, task.TaskID), raw)
				}
				pipe.HSet(ctx, stateKey, encodeSnapshot(updated))
				pipe.XAdd(ctx, &redis.XAddArgs{Stream: eventsKey, ID: fmt.Sprintf("%d-0", updated.EventSeq),
					Values: map[string]any{"type": "agent_run.plan_created", "status": updated.Status, "at": updated.UpdatedAt.Format(time.RFC3339Nano)}})
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

// QueueTask checks dependencies, then atomically changes task/run state,
// records the event, and writes an outbox entry for TaskBus dispatch.
func (s *RedisStore) QueueTask(ctx context.Context, identity Snapshot, expectedVersion, revision uint64, taskID, reason string) (TaskSnapshot, Snapshot, error) {
	if s == nil || s.client == nil || !validScope(identity) || expectedVersion == 0 || revision == 0 || !taskIDPattern.MatchString(taskID) {
		return TaskSnapshot{}, Snapshot{}, ErrInvalid
	}
	stateKey, eventsKey := runKeys(identity)
	_, taskKey, outboxKey := schedulingKeys(identity)
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
			if task.Status != "pending_dependency" && task.Status != "retry_wait" {
				return ErrConflict
			}
			for _, dependency := range task.DependsOn {
				depRaw, err := tx.HGet(ctx, taskKey, taskField(revision, dependency)).Bytes()
				if err != nil {
					return err
				}
				var dep TaskSnapshot
				if err := json.Unmarshal(depRaw, &dep); err != nil {
					return err
				}
				if dep.Status != "completed" {
					return ErrConflict
				}
			}
			task.Status = "queued"
			task.Attempt++
			task.QueuedAt = s.clock().UTC()
			task.QueueReason = reason
			taskRaw, err := json.Marshal(task)
			if err != nil {
				return err
			}
			updated = current
			updated.Status = "running"
			updated.Version++
			updated.EventSeq++
			updated.UpdatedAt = task.QueuedAt
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.HSet(ctx, taskKey, field, taskRaw)
				pipe.HSet(ctx, stateKey, encodeSnapshot(updated))
				pipe.XAdd(ctx, &redis.XAddArgs{Stream: eventsKey, ID: fmt.Sprintf("%d-0", updated.EventSeq),
					Values: map[string]any{"type": "agent_run.task_status", "status": "queued", "task_id": taskID, "attempt": task.Attempt, "at": updated.UpdatedAt.Format(time.RFC3339Nano)}})
				pipe.XAdd(ctx, &redis.XAddArgs{Stream: outboxKey, Values: map[string]any{
					"run_id": identity.RunID, "revision": revision, "task_id": taskID,
					"attempt": task.Attempt, "event_seq": updated.EventSeq,
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

func (s *RedisStore) GetTask(ctx context.Context, identity Snapshot, revision uint64, taskID string) (TaskSnapshot, error) {
	if s == nil || s.client == nil || !validScope(identity) || !validTaskRevision(revision, taskID) {
		return TaskSnapshot{}, ErrInvalid
	}
	if _, err := s.Get(ctx, identity.TenantUUID, identity.Env, identity.RunID); err != nil {
		return TaskSnapshot{}, err
	}
	_, taskKey, _ := schedulingKeys(identity)
	raw, err := s.client.HGet(ctx, taskKey, taskField(revision, taskID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return TaskSnapshot{}, ErrNotFound
	}
	if err != nil {
		return TaskSnapshot{}, err
	}
	var task TaskSnapshot
	err = json.Unmarshal(raw, &task)
	return task, err
}

func validTaskRevision(revision uint64, taskID string) bool {
	return taskIDPattern.MatchString(taskID) && (revision > 0 || taskID == PlanningTaskID)
}

// ListTasks returns the current immutable plan revision's task states for a
// snapshot recovery. Results are sorted so one SSE snapshot is deterministic.
func (s *RedisStore) ListTasks(ctx context.Context, identity Snapshot, revision uint64) ([]TaskSnapshot, error) {
	if s == nil || s.client == nil || !validScope(identity) {
		return nil, ErrInvalid
	}
	_, taskKey, _ := schedulingKeys(identity)
	values, err := s.client.HGetAll(ctx, taskKey).Result()
	if err != nil {
		return nil, err
	}
	tasks := make([]TaskSnapshot, 0)
	for _, raw := range values {
		var task TaskSnapshot
		if err := json.Unmarshal([]byte(raw), &task); err != nil {
			return nil, err
		}
		if task.Revision == revision {
			tasks = append(tasks, task)
		}
	}
	if len(tasks) == 0 && revision > 0 {
		return nil, ErrNotFound
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].TaskID < tasks[j].TaskID })
	return tasks, nil
}

func validPlan(plan Plan) bool {
	if plan.Revision == 0 || len(plan.Tasks) == 0 || len(plan.Tasks) > 1000 {
		return false
	}
	tasks := make(map[string][]string, len(plan.Tasks))
	for _, task := range plan.Tasks {
		if !taskIDPattern.MatchString(task.TaskID) || len(task.DependsOn) > 100 || len(task.PoolID) > 128 {
			return false
		}
		if _, exists := tasks[task.TaskID]; exists {
			return false
		}
		tasks[task.TaskID] = task.DependsOn
	}
	color := make(map[string]uint8, len(tasks))
	var visit func(string) bool
	visit = func(id string) bool {
		if color[id] == 1 {
			return false
		}
		if color[id] == 2 {
			return true
		}
		color[id] = 1
		for _, dep := range tasks[id] {
			if _, exists := tasks[dep]; !exists || dep == id || !visit(dep) {
				return false
			}
		}
		color[id] = 2
		return true
	}
	for id := range tasks {
		if !visit(id) {
			return false
		}
	}
	return true
}

// ValidPlan applies the RunStore DAG, size and dependency limits before a
// caller persists a full execution artifact or publishes its schedule.
func ValidPlan(plan Plan) bool { return validPlan(plan) }

func schedulingKeys(v Snapshot) (string, string, string) {
	stateKey, _ := runKeys(v)
	prefix := stateKey[:len(stateKey)-len(":state")]
	return prefix + ":plans", prefix + ":tasks", prefix + ":outbox"
}

func taskField(revision uint64, taskID string) string {
	return fmt.Sprintf("%d:%s", revision, taskID)
}
