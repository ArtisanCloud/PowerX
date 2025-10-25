package delivery

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"time"

	eventbus "github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

const (
	defaultBaseDelay = 500 * time.Millisecond
	defaultMaxDelay  = 30 * time.Second
	defaultJitter    = 0.25
)

// ScheduleOptions 描述一次重试调度所需的输入参数。
type ScheduleOptions struct {
	TenantKey    string
	SubscriberID string
	EventID      string
	EnvelopeUUID string
	Attempt      int
	BaseDelay    time.Duration
	Metadata     map[string]string
}

// BackoffScheduler 使用 Redis Sorted Set 维护指数退避队列。
type BackoffScheduler struct {
	queue    eventbus.ReliableQueue
	maxDelay time.Duration
	base     time.Duration
	jitter   float64

	now func() time.Time

	randMu sync.Mutex
	random *rand.Rand
}

func NewBackoffScheduler(queue eventbus.ReliableQueue) *BackoffScheduler {
	return &BackoffScheduler{
		queue:    queue,
		maxDelay: defaultMaxDelay,
		base:     defaultBaseDelay,
		jitter:   defaultJitter,
		now:      time.Now,
		random:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Schedule 计算下一次投递时间并写入重试队列。
func (s *BackoffScheduler) Schedule(ctx context.Context, opts ScheduleOptions) (eventbus.RetryItem, error) {
	delay := opts.BaseDelay
	if opts.Attempt <= 0 {
		opts.Attempt = 1
	}

	// Attempt=1 且 BaseDelay=0 时代表立即投递。
	if !(opts.Attempt == 1 && delay == 0) {
		if delay <= 0 {
			delay = s.base
		}
		exp := float64(delay) * math.Pow(2, float64(opts.Attempt-1))
		delay = time.Duration(exp)
		if delay > s.maxDelay {
			delay = s.maxDelay
		}
	}

	if s.jitter > 0 {
		jitterRange := time.Duration(float64(delay) * s.jitter)
		if jitterRange > 0 {
			delay += s.randomJitter(jitterRange)
		}
	}

	executeAt := s.now().Add(delay)
	item := eventbus.RetryItem{
		EventID:      opts.EventID,
		EnvelopeUUID: opts.EnvelopeUUID,
		SubscriberID: opts.SubscriberID,
		TenantKey:    opts.TenantKey,
		Attempt:      opts.Attempt,
		ExecuteAt:    executeAt,
		Backoff:      delay,
		Metadata:     opts.Metadata,
	}
	return item, s.queue.ScheduleRetry(ctx, item)
}

func (s *BackoffScheduler) randomJitter(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	s.randMu.Lock()
	defer s.randMu.Unlock()
	return time.Duration(s.random.Int63n(int64(max)))
}

// PopDue 拉取到期的重试任务。
func (s *BackoffScheduler) PopDue(ctx context.Context, tenantKey string, limit int) ([]eventbus.RetryItem, error) {
	return s.queue.PopDueRetries(ctx, tenantKey, s.now(), limit)
}

// AcquireLease 申请订阅者并发租约，避免超过最大并发。
func (s *BackoffScheduler) AcquireLease(ctx context.Context, lease eventbus.DeliveryLease) (bool, error) {
	return s.queue.AcquireLease(ctx, lease)
}

// ReleaseLease 释放 Ack 租约。
func (s *BackoffScheduler) ReleaseLease(ctx context.Context, lease eventbus.DeliveryLease) error {
	return s.queue.ReleaseLease(ctx, lease)
}

// RemoveRetry 尝试从队列中移除一条记录。
func (s *BackoffScheduler) RemoveRetry(ctx context.Context, item eventbus.RetryItem) error {
	return s.queue.RemoveRetry(ctx, item)
}

// WithClock 允许单元测试覆盖时间来源。
func (s *BackoffScheduler) WithClock(fn func() time.Time) {
	if fn != nil {
		s.now = fn
	}
}
