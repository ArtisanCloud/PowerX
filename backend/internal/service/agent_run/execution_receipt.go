package agent_run

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// ErrExecutionUncertain 表示先前调用可能已经产生副作用，必须核查而非自动重放。
var ErrExecutionUncertain = errors.New("manual_review_required: execution has no durable receipt")

type guardedExecutionKey struct{}

// GuardedExecutionKey 仅由成功占用持久化执行权的 Worker 写入上下文。
func GuardedExecutionKey(ctx context.Context) string {
	key, _ := ctx.Value(guardedExecutionKey{}).(string)
	return key
}

// executionReceipt 在业务调用前占用唯一执行权，回执保存在同一 Task 中。
// 它不使用 TTL；租约到期不能抹去已经开始业务执行的事实。
func (s *RedisStore) executionReceipt(ctx context.Context, ref TaskRef, owner string, fence uint64,
	token string, result *WorkResult) (string, WorkResult, error) {
	identity := Snapshot{TenantUUID: ref.TenantUUID, Env: ref.Env, RunID: ref.RunID}
	if s == nil || s.client == nil || !validScope(identity) || ref.Revision == 0 || ref.Attempt == 0 ||
		!taskIDPattern.MatchString(ref.TaskID) || owner == "" || fence == 0 ||
		(result != nil && (token == "" || strings.TrimSpace(result.ResultRef) == "" || strings.TrimSpace(result.EvidenceRef) == "")) {
		return "", WorkResult{}, ErrInvalid
	}
	claim := uuid.NewString()
	stateKey, _ := runKeys(identity)
	_, taskKey, _ := schedulingKeys(identity)
	for retry := 0; retry < 10; retry++ {
		var receipt WorkResult
		var acquired string
		err := s.client.Watch(ctx, func(tx *redis.Tx) error {
			values, err := tx.HGetAll(ctx, stateKey).Result()
			if err != nil {
				return err
			}
			if len(values) == 0 {
				return ErrNotFound
			}
			run, err := decodeSnapshot(values)
			if err != nil {
				return err
			}
			if terminal(run.Status) || run.PlanRevision != ref.Revision {
				return ErrConflict
			}
			raw, err := tx.HGet(ctx, taskKey, taskField(ref.Revision, ref.TaskID)).Bytes()
			if err != nil {
				return err
			}
			var task TaskSnapshot
			if err := json.Unmarshal(raw, &task); err != nil {
				return err
			}
			if task.TaskID != ref.TaskID || task.Revision != ref.Revision || task.Status != "running" || task.Attempt != ref.Attempt || task.LeaseOwner != owner ||
				task.Fence != fence || !s.clock().Before(task.LeaseUntil) || !s.clock().Before(run.DeadlineAt) {
				return ErrConflict
			}
			if result == nil {
				if task.ExecutionToken != "" {
					if task.ExecutionResultRef == "" || task.ExecutionEvidenceRef == "" {
						return ErrExecutionUncertain
					}
					receipt = WorkResult{ResultRef: task.ExecutionResultRef, EvidenceRef: task.ExecutionEvidenceRef}
					return nil
				}
				task.ExecutionToken = claim
				acquired = claim
			} else {
				if task.ExecutionToken != token {
					return ErrConflict
				}
				if task.ExecutionResultRef != "" && (task.ExecutionResultRef != result.ResultRef || task.ExecutionEvidenceRef != result.EvidenceRef) {
					return ErrConflict
				}
				task.ExecutionResultRef, task.ExecutionEvidenceRef = result.ResultRef, result.EvidenceRef
				receipt = *result
			}
			encoded, err := json.Marshal(task)
			if err != nil {
				return err
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.HSet(ctx, taskKey, taskField(ref.Revision, ref.TaskID), encoded)
				return nil
			})
			return err
		}, stateKey, taskKey)
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		return acquired, receipt, err
	}
	return "", WorkResult{}, ErrConflict
}
