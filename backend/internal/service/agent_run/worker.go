package agent_run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

type WorkResult struct {
	ResultRef   string
	EvidenceRef string
	ReasonCode  string
}

// TaskExecutor must use the stable idempotency key for any side effect and
// persist a business receipt before returning a result reference.
type TaskExecutor interface {
	Execute(context.Context, TaskRef, string) (WorkResult, error)
}

type LeasedTaskQueue interface {
	TaskEnqueuer
	Renew(context.Context, event_bus.StreamTaskDelivery) (event_bus.StreamTaskDelivery, error)
	Ack(context.Context, event_bus.StreamTaskDelivery) error
}

// ProcessDelivery is a single Worker attempt. It never ACKs before the result
// and verification evidence, or a classified failure, is committed to RunStore.
func (s *RedisStore) ProcessDelivery(ctx context.Context, queue LeasedTaskQueue, delivery event_bus.StreamTaskDelivery, executor TaskExecutor) error {
	if s == nil || queue == nil || executor == nil {
		return ErrInvalid
	}
	var ref TaskRef
	if err := json.Unmarshal(delivery.Message.Payload, &ref); err != nil {
		return err
	}
	expectedID := fmt.Sprintf("%s:%d:%s:%d", ref.RunID, ref.Revision, ref.TaskID, ref.Attempt)
	identity := Snapshot{TenantUUID: ref.TenantUUID, Env: ref.Env, RunID: ref.RunID}
	if !validScope(identity) || !taskIDPattern.MatchString(ref.TaskID) ||
		ref.Revision == 0 || ref.Attempt == 0 ||
		delivery.Message.ID != expectedID ||
		delivery.Message.TenantKey != taskQueueTenant(identity) ||
		delivery.Message.SubscriberID != AgentTaskSubscriber ||
		delivery.Lease.MessageID != expectedID {
		return ErrInvalid
	}
	run, err := s.Get(ctx, identity.TenantUUID, identity.Env, identity.RunID)
	if err != nil {
		return err
	}
	task, err := s.GetTask(ctx, identity, ref.Revision, ref.TaskID)
	if err != nil {
		return err
	}
	if task.Attempt != ref.Attempt {
		return ErrConflict
	}
	if task.Status == "completed" || task.Status == "failed" || task.Status == "cancelled" {
		return queue.Ack(ctx, delivery)
	}
	// 同一 Run 的独立任务会同时推进版本；只重试领取/开始的 CAS，
	// 绝不能因为版本竞争重试业务执行。
	for phase := 0; phase < 2; phase++ {
		for retry := 0; retry < 10; retry++ {
			run, err = s.Get(ctx, identity.TenantUUID, identity.Env, identity.RunID)
			if err != nil {
				return err
			}
			if phase == 0 {
				task, run, err = s.LeaseTask(ctx, identity, run.Version, ref.Revision, ref.TaskID,
					delivery.Lease.Owner, delivery.Lease.Token, delivery.Lease.ExpiresAt)
			} else {
				task, run, err = s.StartTask(ctx, identity, run.Version, ref.Revision, ref.TaskID,
					delivery.Lease.Owner, delivery.Lease.Token)
			}
			if !errors.Is(err, ErrConflict) {
				break
			}
		}
		if err != nil {
			return err
		}
	}
	execCtx, cancel := context.WithDeadline(ctx, run.DeadlineAt)
	defer cancel()
	remaining := time.Until(delivery.Lease.ExpiresAt)
	interval := remaining / 3
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
					_, err = s.RenewTaskLease(execCtx, identity, ref.Revision, ref.TaskID,
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
	idempotencyKey := fmt.Sprintf("agent:%s:%d:%s", ref.RunID, ref.Revision, ref.TaskID)
	var result WorkResult
	var execErr error
	if execCtx.Err() != nil {
		execErr = execCtx.Err()
	} else {
		var executionToken string
		executionToken, result, execErr = s.executionReceipt(execCtx, ref, delivery.Lease.Owner, delivery.Lease.Token, "", nil)
		if execErr == nil && executionToken != "" {
			result, execErr = executeWithReceiptGuard(context.WithValue(execCtx, guardedExecutionKey{}, idempotencyKey), executor, ref, idempotencyKey)
			if execErr == nil {
				_, _, execErr = s.executionReceipt(execCtx, ref, delivery.Lease.Owner, delivery.Lease.Token, executionToken, &result)
			}
		}
	}
	close(stop)
	heartbeatErr := <-heartbeatDone
	if heartbeatErr != nil && !errors.Is(heartbeatErr, context.Canceled) &&
		!errors.Is(heartbeatErr, context.DeadlineExceeded) {
		return fmt.Errorf("task lease heartbeat failed: %w", heartbeatErr)
	}
	if execCtx.Err() != nil {
		execErr = execCtx.Err()
		if errors.Is(execErr, context.DeadlineExceeded) {
			result.ReasonCode = "run.deadline"
		} else {
			result.ReasonCode = "manual_review_required"
		}
	}
	if execErr == nil && (strings.TrimSpace(result.ResultRef) == "" || strings.TrimSpace(result.EvidenceRef) == "") {
		execErr = ErrInvalid
		result.ReasonCode = "manual_review_required"
	}
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finishCancel()
	for attempt := 0; attempt < 5; attempt++ {
		run, err = s.Get(finishCtx, identity.TenantUUID, identity.Env, identity.RunID)
		if err != nil {
			return err
		}
		if execErr == nil {
			_, _, err = s.CompleteTask(finishCtx, identity, run.Version, ref.Revision, ref.TaskID,
				delivery.Lease.Owner, delivery.Lease.Token, result.ResultRef, result.EvidenceRef)
		} else {
			reason := strings.TrimSpace(result.ReasonCode)
			if reason == "" {
				reason = "manual_review_required"
			}
			_, _, err = s.FailTask(finishCtx, identity, run.Version, ref.Revision, ref.TaskID,
				delivery.Lease.Owner, delivery.Lease.Token, reason)
		}
		if errors.Is(err, ErrConflict) {
			task, readErr := s.GetTask(finishCtx, identity, ref.Revision, ref.TaskID)
			if readErr != nil || task.Fence != delivery.Lease.Token ||
				task.Status == "completed" || task.Status == "failed" {
				return ErrConflict
			}
			continue
		}
		if err != nil {
			return err
		}
		if err := queue.Ack(finishCtx, delivery); err != nil {
			return err
		}
		advanced, err := s.ReconcilePlan(finishCtx, identity)
		if err != nil {
			return err
		}
		if !terminal(advanced.Status) {
			_, err = s.DispatchPending(finishCtx, identity, queue, 1000)
		}
		return err
	}
	return ErrConflict
}

// 单个业务调用 panic 不得终止 Worker 进程或丢失待核查事实。
func executeWithReceiptGuard(ctx context.Context, executor TaskExecutor, ref TaskRef, key string) (result WorkResult, err error) {
	defer func() {
		if recover() != nil {
			result = WorkResult{ReasonCode: "manual_review_required"}
			err = ErrExecutionUncertain
		}
	}()
	return executor.Execute(ctx, ref, key)
}
