package agent_run

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var (
	ErrInvalid            = errors.New("agent run identity or transition is invalid")
	ErrNotFound           = errors.New("agent run not found")
	ErrConflict           = errors.New("agent run version conflict")
	ErrEventCursorExpired = errors.New("agent run event cursor has expired")
	envPattern            = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,32}$`)
)

// Snapshot contains only scheduling metadata. Prompts, credentials, and result
// bodies belong in access-controlled storage referenced by the task records.
type Snapshot struct {
	TenantUUID   string    `json:"tenant_uuid"`
	Env          string    `json:"env"`
	RunID        string    `json:"run_id"`
	SessionID    string    `json:"session_id"`
	MessageID    string    `json:"message_id"`
	TraceID      string    `json:"trace_id"`
	Status       string    `json:"status"`
	PlanRevision uint64    `json:"plan_revision"`
	Version      uint64    `json:"version"`
	EventSeq     uint64    `json:"event_seq"`
	DeadlineAt   time.Time `json:"deadline_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	ArchiveKey   string    `json:"archive_key,omitempty"`
	ArchivedAt   time.Time `json:"archived_at,omitempty"`
}

type Event struct {
	Seq         uint64    `json:"event_seq"`
	Type        string    `json:"event"`
	Status      string    `json:"status"`
	TaskID      string    `json:"task_id,omitempty"`
	Attempt     uint64    `json:"attempt,omitempty"`
	PoolID      string    `json:"pool_id,omitempty"`
	ReasonCode  string    `json:"reason_code,omitempty"`
	QueueWaitMS int64     `json:"queue_wait_ms,omitempty"`
	CreatedAt   time.Time `json:"occurred_at"`
}

type RedisStore struct {
	client           redis.UniversalClient
	clock            func() time.Time
	queueWaitTimeout time.Duration
}

func NewRedisStore(client redis.UniversalClient) (*RedisStore, error) {
	return NewRedisStoreWithQueueWaitTimeout(client, 10*time.Minute)
}

// NewRedisStoreWithQueueWaitTimeout configures the independent queue-wait
// budget used when a Run is reconciled.
func NewRedisStoreWithQueueWaitTimeout(client redis.UniversalClient, queueWaitTimeout time.Duration) (*RedisStore, error) {
	if client == nil {
		return nil, fmt.Errorf("%w: Redis client is required", ErrInvalid)
	}
	if queueWaitTimeout < time.Second || queueWaitTimeout > 24*time.Hour {
		return nil, fmt.Errorf("%w: queue wait timeout must be between 1s and 24h", ErrInvalid)
	}
	return &RedisStore{client: client, clock: time.Now, queueWaitTimeout: queueWaitTimeout}, nil
}

func (s *RedisStore) Create(ctx context.Context, initial Snapshot) (Snapshot, error) {
	return s.createAdmission(ctx, initial, false)
}

// RecoverAdmission 仅用于已持久化的 pending_create 受理凭据；过期凭据生成失败终态，不执行任务。
func (s *RedisStore) RecoverAdmission(ctx context.Context, initial Snapshot) (Snapshot, error) {
	return s.createAdmission(ctx, initial, true)
}

func (s *RedisStore) createAdmission(ctx context.Context, initial Snapshot, recoverPending bool) (Snapshot, error) {
	if s == nil || s.client == nil || !validIdentity(initial) || initial.Status != "accepted" || initial.DeadlineAt.IsZero() {
		return Snapshot{}, ErrInvalid
	}
	now := s.clock().UTC()
	initial.Version, initial.EventSeq = 1, 1
	initial.CreatedAt, initial.UpdatedAt = now, now
	stateKey, eventsKey := runKeys(initial)
	for attempt := 0; attempt < 5; attempt++ {
		var existing Snapshot
		err := s.client.Watch(ctx, func(tx *redis.Tx) error {
			values, err := tx.HGetAll(ctx, stateKey).Result()
			if err != nil {
				return err
			}
			if len(values) > 0 {
				existing, err = decodeSnapshot(values)
				if err != nil {
					return err
				}
				if existing.SessionID != initial.SessionID || existing.MessageID != initial.MessageID ||
					existing.TraceID != initial.TraceID || !existing.DeadlineAt.Equal(initial.DeadlineAt) {
					return ErrConflict
				}
				return nil
			}
			eventType, reason := "agent_run.started", ""
			if !initial.DeadlineAt.After(now) {
				if !recoverPending {
					return ErrInvalid
				}
				initial.Status, eventType, reason = "failed", "agent_run.failed", "run.deadline"
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.HSet(ctx, stateKey, encodeSnapshot(initial))
				pipe.XAdd(ctx, &redis.XAddArgs{Stream: eventsKey, ID: "1-0", Values: map[string]any{"type": eventType, "status": initial.Status, "reason_code": reason, "at": now.Format(time.RFC3339Nano)}})
				return nil
			})
			return err
		}, stateKey)
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		if err != nil {
			return Snapshot{}, err
		}
		if existing.RunID != "" {
			return existing, nil
		}
		return initial, nil
	}
	return Snapshot{}, ErrConflict
}

func (s *RedisStore) Get(ctx context.Context, tenantUUID, env, runID string) (Snapshot, error) {
	identity := Snapshot{TenantUUID: tenantUUID, Env: env, RunID: runID}
	if s == nil || s.client == nil || !validScope(identity) {
		return Snapshot{}, ErrInvalid
	}
	stateKey, _ := runKeys(identity)
	values, err := s.client.HGetAll(ctx, stateKey).Result()
	if err != nil {
		return Snapshot{}, err
	}
	if len(values) == 0 {
		return Snapshot{}, ErrNotFound
	}
	return decodeSnapshot(values)
}

// Transition updates state and appends the matching event in one Redis
// transaction. The caller must supply the observed version; stale writers
// cannot publish a completion event after another worker has advanced a Run.
func (s *RedisStore) Transition(ctx context.Context, identity Snapshot, expectedVersion uint64, nextStatus, eventType string) (Snapshot, error) {
	if s == nil || s.client == nil || !validScope(identity) || expectedVersion == 0 || !validStatus(nextStatus) || !strings.HasPrefix(eventType, "agent_run.") {
		return Snapshot{}, ErrInvalid
	}
	stateKey, eventsKey := runKeys(identity)
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
			if current.Version != expectedVersion || terminal(current.Status) {
				return ErrConflict
			}
			updated = current
			updated.Status = nextStatus
			updated.Version++
			updated.EventSeq++
			updated.UpdatedAt = s.clock().UTC()
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.HSet(ctx, stateKey, encodeSnapshot(updated))
				pipe.XAdd(ctx, &redis.XAddArgs{Stream: eventsKey, ID: fmt.Sprintf("%d-0", updated.EventSeq), Values: map[string]any{"type": eventType, "status": nextStatus, "at": updated.UpdatedAt.Format(time.RFC3339Nano)}})
				return nil
			})
			return err
		}, stateKey)
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		return updated, err
	}
	return Snapshot{}, ErrConflict
}

func (s *RedisStore) Events(ctx context.Context, tenantUUID, env, runID string, afterSeq uint64, limit int64) ([]Event, error) {
	identity := Snapshot{TenantUUID: tenantUUID, Env: env, RunID: runID}
	if s == nil || s.client == nil || !validScope(identity) || limit <= 0 || limit > 1000 {
		return nil, ErrInvalid
	}
	run, err := s.Get(ctx, tenantUUID, env, runID)
	if err != nil {
		return nil, err
	}
	if afterSeq > run.EventSeq {
		return nil, ErrInvalid
	}
	_, eventsKey := runKeys(identity)
	entries, err := s.client.XRangeN(ctx, eventsKey, fmt.Sprintf("(%d-0", afterSeq), "+", limit).Result()
	if err != nil {
		return nil, err
	}
	if (len(entries) == 0 && run.EventSeq > afterSeq) ||
		(len(entries) > 0 && firstStreamSeq(entries[0].ID) > afterSeq+1) {
		return nil, ErrEventCursorExpired
	}
	result := make([]Event, 0, len(entries))
	expected := afterSeq + 1
	for _, entry := range entries {
		parts := strings.SplitN(entry.ID, "-", 2)
		seq, err := strconv.ParseUint(parts[0], 10, 64)
		if err != nil {
			return nil, err
		}
		if seq != expected {
			return nil, ErrEventCursorExpired
		}
		expected++
		at, err := time.Parse(time.RFC3339Nano, fmt.Sprint(entry.Values["at"]))
		if err != nil {
			return nil, err
		}
		attempt := uint64(0)
		if raw, ok := entry.Values["attempt"]; ok {
			attempt, err = strconv.ParseUint(fmt.Sprint(raw), 10, 64)
			if err != nil {
				return nil, err
			}
		}
		taskID := ""
		if raw, ok := entry.Values["task_id"]; ok {
			taskID = fmt.Sprint(raw)
		}
		poolID := ""
		if raw, ok := entry.Values["pool_id"]; ok {
			poolID = fmt.Sprint(raw)
		}
		reasonCode := ""
		if raw, ok := entry.Values["reason_code"]; ok {
			reasonCode = fmt.Sprint(raw)
		}
		queueWaitMS := int64(0)
		if raw, ok := entry.Values["queue_wait_ms"]; ok {
			queueWaitMS, err = strconv.ParseInt(fmt.Sprint(raw), 10, 64)
			if err != nil {
				return nil, err
			}
		}
		result = append(result, Event{Seq: seq, Type: fmt.Sprint(entry.Values["type"]), Status: fmt.Sprint(entry.Values["status"]), TaskID: taskID, Attempt: attempt, PoolID: poolID, ReasonCode: reasonCode, QueueWaitMS: queueWaitMS, CreatedAt: at})
	}
	return result, nil
}

func firstStreamSeq(id string) uint64 {
	parts := strings.SplitN(id, "-", 2)
	seq, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return 0
	}
	return seq
}

func validScope(v Snapshot) bool {
	tenant, tenantErr := uuid.Parse(v.TenantUUID)
	run, runErr := uuid.Parse(v.RunID)
	return tenantErr == nil && runErr == nil && tenant != uuid.Nil && run != uuid.Nil &&
		tenant.String() == v.TenantUUID && run.String() == v.RunID && envPattern.MatchString(v.Env)
}

func validIdentity(v Snapshot) bool {
	return validScope(v) && strings.TrimSpace(v.SessionID) != "" && strings.TrimSpace(v.MessageID) != "" && strings.TrimSpace(v.TraceID) != ""
}

func validStatus(status string) bool {
	switch status {
	case "accepted", "planning", "running", "completed", "partial", "needs_input", "blocked", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func terminal(status string) bool {
	switch status {
	case "completed", "partial", "needs_input", "blocked", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func runKeys(v Snapshot) (string, string) {
	tag := fmt.Sprintf("{%s:%s:%s}", v.TenantUUID, v.Env, v.RunID)
	return "agent:run:" + tag + ":state", "agent:run:" + tag + ":events"
}

func encodeSnapshot(v Snapshot) map[string]any {
	return map[string]any{
		"tenant_uuid": v.TenantUUID, "env": v.Env, "run_id": v.RunID,
		"session_id": v.SessionID, "message_id": v.MessageID, "trace_id": v.TraceID,
		"status": v.Status, "plan_revision": v.PlanRevision, "version": v.Version, "event_seq": v.EventSeq,
		"deadline_at": v.DeadlineAt.Format(time.RFC3339Nano),
		"created_at":  v.CreatedAt.Format(time.RFC3339Nano),
		"updated_at":  v.UpdatedAt.Format(time.RFC3339Nano),
		"archive_key": v.ArchiveKey,
		"archived_at": formatOptionalTime(v.ArchivedAt),
	}
}

func decodeSnapshot(values map[string]string) (Snapshot, error) {
	version, err := strconv.ParseUint(values["version"], 10, 64)
	if err != nil {
		return Snapshot{}, err
	}
	seq, err := strconv.ParseUint(values["event_seq"], 10, 64)
	if err != nil {
		return Snapshot{}, err
	}
	planRevision := uint64(0)
	if raw := values["plan_revision"]; raw != "" {
		planRevision, err = strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return Snapshot{}, err
		}
	}
	deadline, err := time.Parse(time.RFC3339Nano, values["deadline_at"])
	if err != nil {
		return Snapshot{}, err
	}
	created, err := time.Parse(time.RFC3339Nano, values["created_at"])
	if err != nil {
		return Snapshot{}, err
	}
	updated, err := time.Parse(time.RFC3339Nano, values["updated_at"])
	if err != nil {
		return Snapshot{}, err
	}
	archived := time.Time{}
	if values["archived_at"] != "" {
		archived, err = time.Parse(time.RFC3339Nano, values["archived_at"])
		if err != nil {
			return Snapshot{}, err
		}
	}
	return Snapshot{TenantUUID: values["tenant_uuid"], Env: values["env"], RunID: values["run_id"], SessionID: values["session_id"], MessageID: values["message_id"], TraceID: values["trace_id"], Status: values["status"], PlanRevision: planRevision, Version: version, EventSeq: seq, DeadlineAt: deadline, CreatedAt: created, UpdatedAt: updated, ArchiveKey: values["archive_key"], ArchivedAt: archived}, nil
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
