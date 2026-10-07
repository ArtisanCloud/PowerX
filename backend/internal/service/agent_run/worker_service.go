package agent_run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

// RunLocator 只扫描低频受理锚点；任务状态与心跳一律由 Redis 提供。
type RunLocator interface {
	ListUnfinishedRuns(context.Context, uint64, int) ([]LocatedRun, error)
	FinalizeRun(context.Context, Snapshot) error
}

type RunMaintenance interface{ MaintainRuns(context.Context) error }
type ArchivedRunRecovery interface {
	RecoverArchivedRun(context.Context, Snapshot) (bool, error)
}

type LocatedRun struct {
	Cursor   uint64
	Identity Snapshot
}

type WorkerQueue interface {
	LeasedTaskQueue
	Dequeue(context.Context, string, string, string, string, int64, time.Duration) ([]event_bus.StreamTaskDelivery, error)
}

type capacityDeferredQueue interface {
	Defer(context.Context, event_bus.StreamTaskDelivery, time.Time, string) error
}

// WorkerService 管理消费、恢复扫描与终态回写，生命周期由 Bootstrap 控制。
type WorkerService struct {
	Store        *RedisStore
	Queue        WorkerQueue
	Planner      PlanBuilder
	Executor     TaskExecutor
	Locator      RunLocator
	Concurrency  int
	ScanInterval time.Duration
	Owner        string
	OnError      func(error)
	Capacity     *ExecutionCapacity
}

func (w *WorkerService) Run(ctx context.Context) error {
	if w == nil || w.Store == nil || w.Queue == nil || w.Planner == nil || w.Executor == nil || w.Locator == nil ||
		w.Concurrency < 1 || w.Concurrency > 128 || w.ScanInterval < time.Second || w.Owner == "" {
		return ErrInvalid
	}
	if w.Capacity != nil {
		if _, ok := w.Queue.(capacityDeferredQueue); !ok {
			return fmt.Errorf("execution capacity requires deferred TaskBus")
		}
	}
	slots := make(chan struct{}, w.Concurrency)
	var jobs sync.WaitGroup
	defer jobs.Wait()
	var scopes []Snapshot
	nextScan := time.Time{}
	index := 0
	report := func(err error) {
		if err != nil && !errors.Is(err, context.Canceled) && w.OnError != nil {
			w.OnError(err)
		}
	}
	for ctx.Err() == nil {
		if !time.Now().Before(nextScan) {
			found, err := w.scan(ctx)
			if err == nil {
				scopes = found
				index = 0
			} else {
				report(err)
			}
			nextScan = time.Now().Add(w.ScanInterval)
		}
		if len(scopes) == 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		scope := scopes[index%len(scopes)]
		index++
		deliveries, err := w.Queue.Dequeue(ctx, taskQueueTenant(scope), AgentTaskSubscriber, "agent-run-workers-v1", w.Owner, 1, 10*time.Millisecond)
		if err != nil || len(deliveries) == 0 {
			<-slots
			report(err)
			if err != nil {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(100 * time.Millisecond):
				}
			}
			continue
		}
		jobs.Add(1)
		go func(delivery event_bus.StreamTaskDelivery) {
			defer jobs.Done()
			defer func() { <-slots }()
			var ref TaskRef
			if err := json.Unmarshal(delivery.Message.Payload, &ref); err != nil {
				report(err)
				return
			}
			err := w.process(ctx, ref, delivery)
			if err != nil {
				report(err)
				return
			}
			run, err := w.Store.Get(ctx, ref.TenantUUID, ref.Env, ref.RunID)
			if err == nil && terminal(run.Status) {
				err = w.Locator.FinalizeRun(ctx, run)
			}
			report(err)
		}(deliveries[0])
	}
	return ctx.Err()
}

func (w *WorkerService) process(ctx context.Context, ref TaskRef, delivery event_bus.StreamTaskDelivery) error {
	id := Snapshot{TenantUUID: ref.TenantUUID, Env: ref.Env, RunID: ref.RunID}
	execute := func(execCtx context.Context) error {
		var err error
		if ref.Revision == 0 {
			err = w.Store.ProcessPlanningDelivery(execCtx, w.Queue, delivery, w.Planner)
		} else {
			err = w.Store.ProcessDelivery(execCtx, w.Queue, delivery, w.Executor)
		}
		if errors.Is(err, ErrQueueWaitExpired) {
			// 领取边界重新检查原排队期限，不能抢在低频扫描前执行已经过期的任务。
			if _, recoverErr := w.Store.RecoverRun(execCtx, id, w.Queue); recoverErr != nil {
				return recoverErr
			}
			return w.Queue.Ack(execCtx, delivery)
		}
		return err
	}
	if w.Capacity == nil {
		return execute(ctx)
	}
	if !validScope(id) || delivery.Message.TenantKey != taskQueueTenant(id) {
		return ErrInvalid
	}
	lease, err := w.Capacity.Acquire(ctx, id)
	if errors.Is(err, ErrTenantCapacity) || errors.Is(err, ErrRunCapacity) {
		// 保持原 QueuedAt/attempt；有限等待由既有 queue_wait_timeout 和 Run deadline 治理。
		run, readErr := w.Store.Get(ctx, id.TenantUUID, id.Env, id.RunID)
		if readErr != nil {
			return readErr
		}
		if terminal(run.Status) {
			return w.Queue.Ack(ctx, delivery)
		}
		if !terminal(run.Status) {
			_, _, markErr := w.Store.MarkCapacityWaiting(ctx, id, run.Version, ref.Revision, ref.TaskID, err.Error())
			if markErr != nil && !errors.Is(markErr, ErrConflict) {
				return markErr
			}
		}
		deferred, ok := w.Queue.(capacityDeferredQueue)
		if !ok {
			return ErrInvalid
		}
		return deferred.Defer(ctx, delivery, time.Now().Add(500*time.Millisecond), err.Error())
	}
	if err != nil {
		return err
	}
	return w.Capacity.Execute(ctx, lease, execute)
}

func (w *WorkerService) scan(ctx context.Context) ([]Snapshot, error) {
	if maintenance, ok := w.Locator.(RunMaintenance); ok {
		if err := maintenance.MaintainRuns(ctx); err != nil {
			return nil, err
		}
	}
	scopes := make([]Snapshot, 0)
	seen := make(map[string]bool)
	cursor := uint64(0)
	for {
		rows, err := w.Locator.ListUnfinishedRuns(ctx, cursor, 200)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.Cursor <= cursor || !validScope(row.Identity) {
				return nil, fmt.Errorf("invalid durable admission locator")
			}
			cursor = row.Cursor
			run, err := w.Store.RecoverRun(ctx, row.Identity, w.Queue)
			if errors.Is(err, ErrNotFound) {
				if recovery, ok := w.Locator.(ArchivedRunRecovery); ok {
					handled, recoveryErr := recovery.RecoverArchivedRun(ctx, row.Identity)
					if recoveryErr != nil {
						if w.OnError != nil {
							w.OnError(recoveryErr)
						}
						continue
					}
					if handled {
						continue
					}
				}
			}
			if err != nil {
				// 单个 Run 丢失不应饿死其他租户；记录异常，绝不重建 Redis Run。
				if w.OnError != nil {
					w.OnError(fmt.Errorf("recover run %s: %w", row.Identity.RunID, err))
				}
				continue
			}
			if terminal(run.Status) {
				if err := w.Locator.FinalizeRun(ctx, run); err != nil && w.OnError != nil {
					w.OnError(err)
				}
				continue
			}
			key := taskQueueTenant(run)
			if !seen[key] {
				scopes = append(scopes, run)
				seen[key] = true
			}
		}
		if len(rows) < 200 {
			return scopes, nil
		}
	}
}
