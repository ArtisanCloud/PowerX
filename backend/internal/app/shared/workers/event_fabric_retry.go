package workers

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/delivery"
	sharedsvc "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/shared"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

// TenantProvider 返回需要扫描的租户键列表。
type TenantProvider func(ctx context.Context) ([]string, error)

// EventFabricRetryWorkerOptions 配置重试 Worker。
type EventFabricRetryWorkerOptions struct {
	Delivery                delivery.Service
	EventBus                event_bus.EventBus
	TenantProvider          TenantProvider
	Interval                time.Duration
	BatchSize               int
	EnableDBPollingFallback bool
	DriverName              string
}

// EventFabricRetryWorker 周期拉取重试队列，将事件分发到内部总线。
type EventFabricRetryWorker struct {
	delivery                delivery.Service
	eventBus                event_bus.EventBus
	tenantProvider          TenantProvider
	interval                time.Duration
	batchSize               int
	logger                  *pxlog.Logger
	enableDBPollingFallback bool
	driverName              string
	paused                  atomic.Bool
}

// RetryDispatchEvent 是 worker 推送到 EventBus 的事件载荷。
type RetryDispatchEvent struct {
	TenantKey     string
	SubscriberID  string
	EventID       string
	DeliveryID    string
	EnvelopeID    string
	Version       string
	PayloadFormat string
	TraceID       string
	Payload       []byte
	Headers       map[string]string
	Attempt       int32
	MaxAttempts   int32
	Remaining     int32
	DispatchedAt  time.Time
}

// NewEventFabricRetryWorker 构建重试 worker。
func NewEventFabricRetryWorker(opts EventFabricRetryWorkerOptions) *EventFabricRetryWorker {
	interval := opts.Interval
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = 50
	}
	return &EventFabricRetryWorker{
		delivery:                opts.Delivery,
		eventBus:                opts.EventBus,
		tenantProvider:          opts.TenantProvider,
		interval:                interval,
		batchSize:               batchSize,
		logger:                  pxlog.GetGlobalLogger(),
		enableDBPollingFallback: opts.EnableDBPollingFallback,
		driverName:              strings.TrimSpace(opts.DriverName),
	}
}

// Run 启动后台轮询。
func (w *EventFabricRetryWorker) Run(ctx context.Context) {
	if w.delivery == nil || w.eventBus == nil || w.tenantProvider == nil {
		return
	}
	if !w.enableDBPollingFallback {
		w.logger.InfoF(ctx, "[event_fabric.retry] skip db polling fallback driver=%s reason=fallback_disabled", w.driverName)
		return
	}

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			if !w.paused.Load() {
				w.flush(ctx)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *EventFabricRetryWorker) Pause() {
	if w == nil {
		return
	}
	w.paused.Store(true)
}

func (w *EventFabricRetryWorker) Resume() {
	if w == nil {
		return
	}
	w.paused.Store(false)
}

func (w *EventFabricRetryWorker) IsPaused() bool {
	if w == nil {
		return false
	}
	return w.paused.Load()
}

func (w *EventFabricRetryWorker) TriggerNow(ctx context.Context) {
	if w == nil || w.delivery == nil || w.eventBus == nil || w.tenantProvider == nil {
		return
	}
	w.flush(ctx)
}

func (w *EventFabricRetryWorker) Interval() time.Duration {
	if w == nil {
		return 0
	}
	return w.interval
}

func (w *EventFabricRetryWorker) BatchSize() int {
	if w == nil {
		return 0
	}
	return w.batchSize
}

func (w *EventFabricRetryWorker) flush(ctx context.Context) {
	tenantKeys, err := w.tenantProvider(ctx)
	if err != nil {
		w.logger.WarnF(ctx, "[event_fabric.retry] tenant provider failed: %v", err)
		return
	}
	if len(tenantKeys) == 0 {
		return
	}
	for _, tenant := range tenantKeys {
		tenant = strings.TrimSpace(tenant)
		if tenant == "" {
			continue
		}
		w.drainTenant(ctx, tenant)
	}
}

func (w *EventFabricRetryWorker) drainTenant(ctx context.Context, tenantKey string) {
	pollCtx := context.WithValue(ctx, sharedsvc.ContextTenantKey, tenantKey)

	for {
		events, err := w.delivery.PollRetry(pollCtx, w.batchSize)
		if err != nil {
			if !errors.Is(err, sharedsvc.ErrTenantMismatch) {
				w.logger.WarnF(ctx, "[event_fabric.retry] poll failed tenant=%s err=%v", tenantKey, err)
			}
			return
		}
		if len(events) == 0 {
			return
		}

		for _, attempts := range events {
			for _, attempt := range attempts {
				w.dispatchAttempt(pollCtx, tenantKey, attempt)
			}
		}
	}
}

func (w *EventFabricRetryWorker) dispatchAttempt(ctx context.Context, tenantKey string, attempt delivery.DeliveryAttempt) {
	event := RetryDispatchEvent{
		TenantKey:     tenantKey,
		SubscriberID:  attempt.SubscriberID,
		EventID:       attempt.EventID,
		DeliveryID:    attempt.DeliveryUUID,
		EnvelopeID:    attempt.EnvelopeUUID,
		Version:       attempt.Version,
		PayloadFormat: attempt.PayloadFormat,
		TraceID:       attempt.TraceID,
		Payload:       attempt.Payload,
		Headers:       attempt.Headers,
		Attempt:       attempt.AttemptNumber,
		MaxAttempts:   attempt.MaxAttempts,
		Remaining:     attempt.Remaining,
		DispatchedAt:  time.Now(),
	}

	dispatchCtx := ctx
	if attempt.TraceID != "" {
		dispatchCtx = context.WithValue(dispatchCtx, "trace_id", attempt.TraceID)
	}
	w.eventBus.Publish("event_fabric.retry.dispatch", event, dispatchCtx)

	if err := w.delivery.Ack(ctx, attempt.DeliveryUUID, attempt.SubscriberID); err != nil {
		w.logger.WarnF(ctx, "[event_fabric.retry] ack failed tenant=%s delivery=%s err=%v", tenantKey, attempt.DeliveryUUID, err)
	}
}
