package workflow

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"

	eventbus "github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

// RetryScheduleOptions 描述一次重试调度的最小输入。
type RetryScheduleOptions struct {
	TenantKey    string
	WorkerID     string
	InstanceUUID uuid.UUID
	StepRecordID uint64
	Attempt      int
	Delay        time.Duration
	Metadata     map[string]string
}

// RetryPolicyDefinition 描述重试策略参数。
type RetryPolicyDefinition struct {
	MaxAttempts       int
	InitialInterval   time.Duration
	BackoffMultiplier float64
	MaxInterval       time.Duration
	Jitter            time.Duration
}

// Scheduler 封装 Redis 队列的调度与租约操作。
type Scheduler struct {
	queue eventbus.ReliableQueue
	now   func() time.Time
}

// NewScheduler 创建调度器实例。
func NewScheduler(queue eventbus.ReliableQueue) *Scheduler {
	return &Scheduler{
		queue: queue,
		now:   time.Now,
	}
}

// ScheduleRetry 将任务推入重试队列。
func (s *Scheduler) ScheduleRetry(ctx context.Context, opts RetryScheduleOptions) (eventbus.RetryItem, error) {
	if opts.Attempt <= 0 {
		opts.Attempt = 1
	}
	delay := opts.Delay
	if delay < 0 {
		delay = 0
	}

	item := eventbus.RetryItem{
		EventID:      opts.InstanceUUID.String(),
		EnvelopeUUID: fmt.Sprintf("%s:%d", opts.InstanceUUID.String(), opts.StepRecordID),
		SubscriberID: opts.WorkerID,
		TenantKey:    opts.TenantKey,
		Attempt:      opts.Attempt,
		ExecuteAt:    s.now().Add(delay),
		Backoff:      delay,
		Metadata:     opts.Metadata,
	}

	return item, s.queue.ScheduleRetry(ctx, item)
}

// NextDelay 根据策略计算下一次重试的延迟。
func (s *Scheduler) NextDelay(policy RetryPolicyDefinition, attempt int) time.Duration {
	if attempt <= 1 {
		return clampInterval(policy.InitialInterval, policy)
	}
	interval := float64(policy.InitialInterval)
	if interval <= 0 {
		interval = float64(30 * time.Second)
	}
	backoff := policy.BackoffMultiplier
	if backoff <= 1 {
		backoff = 2
	}
	next := time.Duration(interval * math.Pow(backoff, float64(attempt-1)))
	return clampInterval(next, policy)
}

func clampInterval(interval time.Duration, policy RetryPolicyDefinition) time.Duration {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if policy.MaxInterval > 0 && interval > policy.MaxInterval {
		interval = policy.MaxInterval
	}
	return interval
}

// PopDue 拉取到期的重试任务。
func (s *Scheduler) PopDue(ctx context.Context, tenantKey string, limit int) ([]eventbus.RetryItem, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.queue.PopDueRetries(ctx, tenantKey, s.now(), limit)
}

// RemoveRetry 从队列移除指定任务。
func (s *Scheduler) RemoveRetry(ctx context.Context, item eventbus.RetryItem) error {
	return s.queue.RemoveRetry(ctx, item)
}

// AcquireLease 申请处理租约。
func (s *Scheduler) AcquireLease(ctx context.Context, lease eventbus.DeliveryLease) (bool, error) {
	return s.queue.AcquireLease(ctx, lease)
}

// ReleaseLease 释放已处理或超时的租约。
func (s *Scheduler) ReleaseLease(ctx context.Context, lease eventbus.DeliveryLease) error {
	return s.queue.ReleaseLease(ctx, lease)
}

// WithClock 允许测试覆盖时间。
func (s *Scheduler) WithClock(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}
