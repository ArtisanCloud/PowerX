package workers

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"time"

	eventdomain "github.com/ArtisanCloud/PowerX/internal/event_bus"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

const (
	defaultSystemNotificationDispatchSubscriber = eventdomain.SubscriberSystemNotificationDispatch
	defaultSystemNotificationDispatchTenant     = "global"
)

type notificationPublisher func(tenantKey, topic string, payload any, traceID string)

type EventFabricSystemNotificationDispatchWorkerOptions struct {
	TaskDriver   event_bus.TaskDriver
	SubscriberID string
	TenantKey    string
	BatchSize    int
	WaitTimeout  time.Duration
	RetryDelay   time.Duration
	Publish      notificationPublisher
	Logger       *pxlog.Logger
	Clock        func() time.Time
}

type EventFabricSystemNotificationDispatchWorker struct {
	taskDriver   event_bus.TaskDriver
	subscriberID string
	tenantKey    string
	batchSize    int
	waitTimeout  time.Duration
	retryDelay   time.Duration
	publish      notificationPublisher
	logger       *pxlog.Logger
	clock        func() time.Time
	paused       atomic.Bool
}

func NewEventFabricSystemNotificationDispatchWorker(opts EventFabricSystemNotificationDispatchWorkerOptions) *EventFabricSystemNotificationDispatchWorker {
	if opts.TaskDriver == nil || opts.Publish == nil {
		return nil
	}
	subscriberID := strings.TrimSpace(opts.SubscriberID)
	if subscriberID == "" {
		subscriberID = defaultSystemNotificationDispatchSubscriber
	}
	tenantKey := strings.TrimSpace(opts.TenantKey)
	if tenantKey == "" {
		tenantKey = defaultSystemNotificationDispatchTenant
	}
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = 50
	}
	waitTimeout := opts.WaitTimeout
	if waitTimeout <= 0 {
		waitTimeout = 3 * time.Second
	}
	retryDelay := opts.RetryDelay
	if retryDelay <= 0 {
		retryDelay = 5 * time.Second
	}
	logger := opts.Logger
	if logger == nil {
		logger = pxlog.GetGlobalLogger()
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	return &EventFabricSystemNotificationDispatchWorker{
		taskDriver:   opts.TaskDriver,
		subscriberID: subscriberID,
		tenantKey:    tenantKey,
		batchSize:    batchSize,
		waitTimeout:  waitTimeout,
		retryDelay:   retryDelay,
		publish:      opts.Publish,
		logger:       logger,
		clock:        clock,
	}
}

func (w *EventFabricSystemNotificationDispatchWorker) Run(ctx context.Context) {
	if w == nil || w.taskDriver == nil || w.publish == nil {
		return
	}
	for {
		if ctx.Err() != nil {
			return
		}
		if w.paused.Load() {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		messages, err := w.taskDriver.Dequeue(ctx, event_bus.DequeueRequest{
			TenantKey:    w.tenantKey,
			SubscriberID: w.subscriberID,
			MaxItems:     w.batchSize,
			WaitTimeout:  w.waitTimeout,
		})
		if err != nil {
			w.logger.WarnF(ctx, "[system.notification.task] dequeue failed: %v", err)
			time.Sleep(time.Second)
			continue
		}
		if len(messages) == 0 {
			continue
		}
		for _, msg := range messages {
			w.handleMessage(ctx, msg)
		}
	}
}

func (w *EventFabricSystemNotificationDispatchWorker) Pause() {
	if w == nil {
		return
	}
	w.paused.Store(true)
}

func (w *EventFabricSystemNotificationDispatchWorker) Resume() {
	if w == nil {
		return
	}
	w.paused.Store(false)
}

func (w *EventFabricSystemNotificationDispatchWorker) IsPaused() bool {
	if w == nil {
		return false
	}
	return w.paused.Load()
}

func (w *EventFabricSystemNotificationDispatchWorker) SubscriberID() string {
	if w == nil {
		return ""
	}
	return w.subscriberID
}

func (w *EventFabricSystemNotificationDispatchWorker) TenantKey() string {
	if w == nil {
		return ""
	}
	return w.tenantKey
}

func (w *EventFabricSystemNotificationDispatchWorker) handleMessage(ctx context.Context, msg event_bus.TaskMessage) {
	var payload map[string]any
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		_ = w.taskDriver.Ack(ctx, event_bus.AckRequest{TenantKey: w.tenantKey, SubscriberID: w.subscriberID, MessageID: msg.ID})
		return
	}

	topic := strings.TrimSpace(msg.Topic)
	if topic == "" {
		topic = eventdomain.TopicSystemNotification
	}
	publishTenantKey := strings.TrimSpace(msg.Metadata["tenant_uuid"])
	if publishTenantKey == "" {
		publishTenantKey = strings.TrimSpace(msg.TenantKey)
	}
	if publishTenantKey == "" {
		publishTenantKey = w.tenantKey
	}
	w.publish(publishTenantKey, topic, payload, strings.TrimSpace(msg.TraceID))

	if err := w.taskDriver.Ack(ctx, event_bus.AckRequest{
		TenantKey:    w.tenantKey,
		SubscriberID: w.subscriberID,
		MessageID:    msg.ID,
	}); err != nil {
		w.logger.WarnF(ctx, "[system.notification.task] ack failed id=%s err=%v", msg.ID, err)
		return
	}
}
