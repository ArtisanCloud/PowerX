package agent_run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/redis/go-redis/v9"
)

const AgentTaskSubscriber = "agent-run"

type TaskRef struct {
	TenantUUID string `json:"tenant_uuid"`
	Env        string `json:"env"`
	RunID      string `json:"run_id"`
	Revision   uint64 `json:"revision"`
	TaskID     string `json:"task_id"`
	Attempt    uint64 `json:"attempt"`
}

type TaskEnqueuer interface {
	Enqueue(context.Context, event_bus.TaskMessage) error
}

// DispatchPending bridges the per-Run atomic outbox to the shared TaskBus.
// Cursor advancement follows successful enqueue; replay is safe because the
// TaskBus deduplicates the stable message ID.
func (s *RedisStore) DispatchPending(ctx context.Context, identity Snapshot, queue TaskEnqueuer, limit int64) (int, error) {
	if s == nil || s.client == nil || queue == nil || !validScope(identity) || limit < 1 || limit > 1000 {
		return 0, ErrInvalid
	}
	run, err := s.Get(ctx, identity.TenantUUID, identity.Env, identity.RunID)
	if err != nil {
		return 0, err
	}
	if terminal(run.Status) {
		return 0, ErrConflict
	}
	_, _, outboxKey := schedulingKeys(identity)
	cursorKey := strings.TrimSuffix(outboxKey, ":outbox") + ":dispatch_cursor"
	cursor, err := s.client.Get(ctx, cursorKey).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			return 0, err
		}
		cursor = "0-0"
	}
	entries, err := s.client.XRangeN(ctx, outboxKey, "("+cursor, "+", limit).Result()
	if err != nil {
		return 0, err
	}
	dispatched := 0
	for _, entry := range entries {
		revision, err := strconv.ParseUint(fmt.Sprint(entry.Values["revision"]), 10, 64)
		if err != nil {
			return dispatched, err
		}
		attempt, err := strconv.ParseUint(fmt.Sprint(entry.Values["attempt"]), 10, 64)
		if err != nil {
			return dispatched, err
		}
		ref := TaskRef{
			TenantUUID: identity.TenantUUID, Env: identity.Env, RunID: identity.RunID,
			Revision: revision, TaskID: fmt.Sprint(entry.Values["task_id"]), Attempt: attempt,
		}
		if !taskIDPattern.MatchString(ref.TaskID) || fmt.Sprint(entry.Values["run_id"]) != identity.RunID {
			return dispatched, ErrInvalid
		}
		raw, err := json.Marshal(ref)
		if err != nil {
			return dispatched, err
		}
		message := event_bus.TaskMessage{
			ID:        fmt.Sprintf("%s:%d:%s:%d", ref.RunID, ref.Revision, ref.TaskID, ref.Attempt),
			TenantKey: taskQueueTenant(identity), SubscriberID: AgentTaskSubscriber,
			Topic: "agent.run.task", Payload: raw,
		}
		if err := queue.Enqueue(ctx, message); err != nil {
			return dispatched, err
		}
		if err := s.client.Set(ctx, cursorKey, entry.ID, 0).Err(); err != nil {
			return dispatched, err
		}
		dispatched++
	}
	return dispatched, nil
}

func taskQueueTenant(identity Snapshot) string {
	return identity.TenantUUID + ":" + identity.Env
}
