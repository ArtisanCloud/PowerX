package workers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	eventdomain "github.com/ArtisanCloud/PowerX/internal/event_bus"
	authsvc "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/authorization"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"

	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

const (
	defaultAuthorizationTimeoutTaskSubscriber = eventdomain.SubscriberAuthorizationChallengeTime
	defaultAuthorizationTimeoutTaskTenant     = "global"
)

type authorizationTimeoutTaskPayload struct {
	TicketID string `json:"ticket_id"`
}

type EventFabricAuthorizationTimeoutTaskWorkerOptions struct {
	Service      authsvc.Service
	TaskDriver   event_bus.TaskDriver
	SubscriberID string
	TenantKey    string
	BatchSize    int
	WaitTimeout  time.Duration
	RetryDelay   time.Duration
	Logger       *pxlog.Logger
	Clock        func() time.Time
}

type EventFabricAuthorizationTimeoutTaskWorker struct {
	service      authsvc.Service
	taskDriver   event_bus.TaskDriver
	subscriberID string
	tenantKey    string
	batchSize    int
	waitTimeout  time.Duration
	retryDelay   time.Duration
	logger       *pxlog.Logger
	clock        func() time.Time
	paused       atomic.Bool
}

func NewEventFabricAuthorizationTimeoutTaskWorker(opts EventFabricAuthorizationTimeoutTaskWorkerOptions) *EventFabricAuthorizationTimeoutTaskWorker {
	if opts.Service == nil || opts.TaskDriver == nil {
		return nil
	}
	subscriberID := strings.TrimSpace(opts.SubscriberID)
	if subscriberID == "" {
		subscriberID = defaultAuthorizationTimeoutTaskSubscriber
	}
	tenantKey := strings.TrimSpace(opts.TenantKey)
	if tenantKey == "" {
		tenantKey = defaultAuthorizationTimeoutTaskTenant
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
	return &EventFabricAuthorizationTimeoutTaskWorker{
		service:      opts.Service,
		taskDriver:   opts.TaskDriver,
		subscriberID: subscriberID,
		tenantKey:    tenantKey,
		batchSize:    batchSize,
		waitTimeout:  waitTimeout,
		retryDelay:   retryDelay,
		logger:       logger,
		clock:        clock,
	}
}

func (w *EventFabricAuthorizationTimeoutTaskWorker) Run(ctx context.Context) {
	if w == nil || w.service == nil || w.taskDriver == nil {
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
			w.logger.WarnF(ctx, "[authorization.timeout.task] dequeue failed: %v", err)
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

func (w *EventFabricAuthorizationTimeoutTaskWorker) Pause() {
	if w == nil {
		return
	}
	w.paused.Store(true)
}

func (w *EventFabricAuthorizationTimeoutTaskWorker) Resume() {
	if w == nil {
		return
	}
	w.paused.Store(false)
}

func (w *EventFabricAuthorizationTimeoutTaskWorker) IsPaused() bool {
	if w == nil {
		return false
	}
	return w.paused.Load()
}

func (w *EventFabricAuthorizationTimeoutTaskWorker) TriggerNow(ctx context.Context) {
	if w == nil || w.service == nil || w.taskDriver == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	messages, err := w.taskDriver.Dequeue(ctx, event_bus.DequeueRequest{
		TenantKey:    w.tenantKey,
		SubscriberID: w.subscriberID,
		MaxItems:     w.batchSize,
		WaitTimeout:  0,
	})
	if err != nil || len(messages) == 0 {
		return
	}
	for _, msg := range messages {
		w.handleMessage(ctx, msg)
	}
}

func (w *EventFabricAuthorizationTimeoutTaskWorker) SubscriberID() string {
	if w == nil {
		return ""
	}
	return w.subscriberID
}

func (w *EventFabricAuthorizationTimeoutTaskWorker) TenantKey() string {
	if w == nil {
		return ""
	}
	return w.tenantKey
}

func (w *EventFabricAuthorizationTimeoutTaskWorker) BatchSize() int {
	if w == nil {
		return 0
	}
	return w.batchSize
}

func (w *EventFabricAuthorizationTimeoutTaskWorker) WaitTimeout() time.Duration {
	if w == nil {
		return 0
	}
	return w.waitTimeout
}

func (w *EventFabricAuthorizationTimeoutTaskWorker) handleMessage(ctx context.Context, msg event_bus.TaskMessage) {
	ticketID, err := decodeAuthorizationTimeoutTaskPayload(msg.Payload)
	if err != nil {
		_ = w.taskDriver.Ack(ctx, event_bus.AckRequest{TenantKey: w.tenantKey, SubscriberID: w.subscriberID, MessageID: msg.ID})
		return
	}

	_, processErr := w.service.ProcessExpiredChallengeTicket(ctx, ticketID, w.clock().UTC())
	if processErr == nil || errors.Is(processErr, authsvc.ErrChallengeNotFound) || errors.Is(processErr, authsvc.ErrChallengeResolved) {
		if err := w.taskDriver.Ack(ctx, event_bus.AckRequest{TenantKey: w.tenantKey, SubscriberID: w.subscriberID, MessageID: msg.ID}); err != nil {
			w.logger.WarnF(ctx, "[authorization.timeout.task] ack failed ticket=%s err=%v", ticketID, err)
		}
		return
	}

	retryAt := w.clock().UTC().Add(w.retryDelay)
	if err := w.taskDriver.Nack(ctx, event_bus.NackRequest{
		TenantKey:    w.tenantKey,
		SubscriberID: w.subscriberID,
		MessageID:    msg.ID,
		Reason:       processErr.Error(),
		RetryAt:      retryAt,
	}); err != nil {
		w.logger.WarnF(ctx, "[authorization.timeout.task] nack failed ticket=%s err=%v", ticketID, err)
	}
}

func RegisterAuthorizationChallengeTimeoutTaskScheduler(eventBus event_bus.EventBus, taskDriver event_bus.TaskDriver, topic string, logger *pxlog.Logger, clock func() time.Time) (unsubscribe func()) {
	if eventBus == nil || taskDriver == nil {
		return func() {}
	}
	if logger == nil {
		logger = pxlog.GetGlobalLogger()
	}
	if clock == nil {
		clock = time.Now
	}
	topic = strings.TrimSpace(topic)
	if topic == "" {
		topic = "event_fabric.authorization.challenge"
	}

	return eventBus.Subscribe(topic, func(evt event_bus.Event) error {
		payloadBytes, err := json.Marshal(evt.Payload)
		if err != nil {
			return err
		}
		var challenge authsvc.ChallengeEvent
		if err := json.Unmarshal(payloadBytes, &challenge); err != nil {
			return err
		}
		if !strings.EqualFold(strings.TrimSpace(challenge.Type), "queued") {
			return nil
		}
		if challenge.TicketID == uuid.Nil {
			return nil
		}

		visibleAt := challenge.SLAExpiresAt.UTC()
		if visibleAt.IsZero() {
			visibleAt = clock().UTC()
		}
		taskPayload, err := json.Marshal(authorizationTimeoutTaskPayload{TicketID: challenge.TicketID.String()})
		if err != nil {
			return err
		}
		msgID := fmt.Sprintf("authorization.timeout.%s.%d", challenge.TicketID.String(), visibleAt.UnixMilli())
		return taskDriver.Enqueue(evt.Ctx, event_bus.TaskMessage{
			ID:           msgID,
			TenantKey:    defaultAuthorizationTimeoutTaskTenant,
			SubscriberID: defaultAuthorizationTimeoutTaskSubscriber,
			Topic:        topic,
			Payload:      taskPayload,
			TraceID:      challenge.RequestFingerprint.String(),
			VisibleAt:    visibleAt,
			Metadata: map[string]string{
				"ticket_id": challenge.TicketID.String(),
				"event":     "challenge_timeout",
			},
		})
	})
}

func decodeAuthorizationTimeoutTaskPayload(payload []byte) (uuid.UUID, error) {
	if len(payload) == 0 {
		return uuid.Nil, fmt.Errorf("empty payload")
	}
	var body authorizationTimeoutTaskPayload
	if err := json.Unmarshal(payload, &body); err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(strings.TrimSpace(body.TicketID))
}
