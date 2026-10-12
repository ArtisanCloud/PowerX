package agent_run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cancel fences every unfinished task in the same transaction as the Run.
// It stops scheduling, but does not assert that external side effects were undone.
func (s *RedisStore) Cancel(ctx context.Context, identity Snapshot) (Snapshot, error) {
	if s == nil || s.client == nil || !validScope(identity) {
		return Snapshot{}, ErrInvalid
	}
	stateKey, eventsKey := runKeys(identity)
	_, taskKey, outboxKey := schedulingKeys(identity)
	for retry := 0; retry < 10; retry++ {
		var run Snapshot
		err := s.client.Watch(ctx, func(tx *redis.Tx) error {
			values, err := tx.HGetAll(ctx, stateKey).Result()
			if err != nil {
				return err
			}
			if len(values) == 0 {
				return ErrNotFound
			}
			run, err = decodeSnapshot(values)
			if err != nil {
				return err
			}
			if terminal(run.Status) {
				return nil
			}
			rows, err := tx.HGetAll(ctx, taskKey).Result()
			if err != nil {
				return err
			}
			fields := make([]string, 0, len(rows))
			changed := map[string]TaskSnapshot{}
			now := s.clock().UTC()
			for field, raw := range rows {
				var task TaskSnapshot
				if err := json.Unmarshal([]byte(raw), &task); err != nil {
					return err
				}
				switch task.Status {
				case "completed", "failed", "skipped", "cancelled":
					continue
				}
				task.Status, task.CompletedAt, task.LeaseUntil = "cancelled", now, now
				task.ReasonCode = "run.cancelled"
				if task.ExecutionToken != "" && (task.ExecutionResultRef == "" || task.ExecutionEvidenceRef == "") {
					task.ReasonCode = "manual_review_required"
				}
				changed[field] = task
				fields = append(fields, field)
			}
			sort.Strings(fields)
			run.Status, run.UpdatedAt = "cancelled", now
			run.Version++
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				for _, field := range fields {
					task := changed[field]
					raw, err := json.Marshal(task)
					if err != nil {
						return err
					}
					pipe.HSet(ctx, taskKey, field, raw)
					run.EventSeq++
					pipe.XAdd(ctx, &redis.XAddArgs{Stream: eventsKey, ID: fmt.Sprintf("%d-0", run.EventSeq), Values: map[string]any{"type": "agent_run.task_cancelled", "status": "cancelled", "task_id": task.TaskID, "attempt": task.Attempt, "reason_code": task.ReasonCode, "at": now.Format(time.RFC3339Nano)}})
				}
				run.EventSeq++
				pipe.HSet(ctx, stateKey, encodeSnapshot(run))
				pipe.Del(ctx, outboxKey)
				pipe.XAdd(ctx, &redis.XAddArgs{Stream: eventsKey, ID: fmt.Sprintf("%d-0", run.EventSeq), Values: map[string]any{"type": "agent_run.cancelled", "status": "cancelled", "at": now.Format(time.RFC3339Nano)}})
				return nil
			})
			return err
		}, stateKey, taskKey, outboxKey)
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		return run, err
	}
	return Snapshot{}, ErrConflict
}
