package delivery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	eventaudit "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/audit"
	eventmetrics "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/metrics"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/shared"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	eventfabricrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	eventbus "github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// PublishRequest 统一发布事件所需字段。
type PublishRequest struct {
	TenantUUID     string
	Topic          string
	EventID        string
	TraceID        string
	Version        string
	Payload        []byte
	PayloadFormat  string
	IdempotencyKey string
	Attributes     map[string]string
}

// DeliveryAttempt 描述一次投递尝试的状态。
type DeliveryAttempt struct {
	AttemptNumber int32
	SubscriberID  string
	StartedAt     time.Time
	CompletedAt   *time.Time
	Status        string
	ErrorMessage  string

	EventID       string
	DeliveryUUID  string
	EnvelopeUUID  string
	Payload       []byte
	Headers       map[string]string
	Version       string
	PayloadFormat string
	TraceID       string
	TopicFullName string
	AckTimeout    time.Duration
	MaxAttempts   int32
	Remaining     int32
}

// RetryPlan 描述当前事件的重试策略。
type RetryPlan struct {
	MaxAttempts       int32
	RemainingAttempts int32
	NextDelay         time.Duration
	Strategy          string
}

// Service 是事件投递与重试 orchestrator 的接口。
type Service interface {
	Publish(ctx context.Context, req PublishRequest) error
	Ack(ctx context.Context, deliveryID string, subscriberID string) error
	Nack(ctx context.Context, deliveryID string, subscriberID string, reason string) (RetryPlan, error)
	PollRetry(ctx context.Context, limit int) (map[string][]DeliveryAttempt, error)
}

type envelopeStore interface {
	UpsertByEventID(ctx context.Context, envelope *eventfabricmodel.EventEnvelope) (*eventfabricmodel.EventEnvelope, bool, error)
	FindByEventID(ctx context.Context, tenantKey, eventID string) (*eventfabricmodel.EventEnvelope, error)
	FindByUUID(ctx context.Context, id uuid.UUID) (*eventfabricmodel.EventEnvelope, error)
	UpdateStatus(ctx context.Context, envelopeUUID uuid.UUID, updates map[string]interface{}) error
}

type deliveryStore interface {
	UpsertAttempt(ctx context.Context, attempt *eventfabricmodel.DeliveryAttempt) (*eventfabricmodel.DeliveryAttempt, error)
	FindByEnvelopeAndSubscriber(ctx context.Context, envelope uuid.UUID, subscriberID string) (*eventfabricmodel.DeliveryAttempt, error)
	FindByUUID(ctx context.Context, id uuid.UUID) (*eventfabricmodel.DeliveryAttempt, error)
	UpdateStatus(ctx context.Context, attemptUUID uuid.UUID, updates map[string]interface{}) error
	CountActiveAttempts(ctx context.Context, envelope uuid.UUID) (int64, error)
}

type dlqStore interface {
	Create(ctx context.Context, message *eventfabricmodel.DlqMessage) (*eventfabricmodel.DlqMessage, error)
}

type topicStore interface {
	FindByComposite(ctx context.Context, tenantKey, namespace, name string) (*eventfabricmodel.TopicDefinition, error)
	FindByUUID(ctx context.Context, id uuid.UUID) (*eventfabricmodel.TopicDefinition, error)
}

type aclStore interface {
	HasPermission(ctx context.Context, tenantKey string, topic uuid.UUID, principalID string, action string, now time.Time) (bool, error)
	ListByTopic(ctx context.Context, tenantKey string, topic uuid.UUID) ([]*eventfabricmodel.AclBinding, error)
}

// Options 汇总 Service 所需依赖。
type Options struct {
	DB                           *gorm.DB
	Envelopes                    envelopeStore
	Deliveries                   deliveryStore
	DLQ                          dlqStore
	Topics                       topicStore
	ACL                          aclStore
	Audit                        eventaudit.Service
	Scheduler                    *BackoffScheduler
	Clock                        func() time.Time
	MaxRetry                     int
	Negotiator                   VersionNegotiator
	Metrics                      eventmetrics.Recorder
	EnableDatabaseFallbackLookup bool
}

type serviceImpl struct {
	db                           *gorm.DB
	envelopes                    envelopeStore
	deliveries                   deliveryStore
	dlq                          dlqStore
	topics                       topicStore
	acl                          aclStore
	scheduler                    *BackoffScheduler
	clock                        func() time.Time
	maxRetry                     int
	audit                        eventaudit.Service
	negotiator                   VersionNegotiator
	metrics                      eventmetrics.Recorder
	enableDatabaseFallbackLookup bool
}

// NewService 构建事件投递服务。
func NewService(opts Options) (Service, error) {
	if opts.Scheduler == nil {
		return nil, fmt.Errorf("scheduler is required")
	}

	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}

	maxRetry := opts.MaxRetry
	if maxRetry <= 0 {
		maxRetry = shared.DefaultMaxRetry
	}

	envStore := opts.Envelopes
	if envStore == nil {
		if opts.DB == nil {
			return nil, fmt.Errorf("envelope store requires db")
		}
		envStore = eventfabricrepo.NewEnvelopeRepository(opts.DB)
	}

	deliverStore := opts.Deliveries
	if deliverStore == nil {
		if opts.DB == nil {
			return nil, fmt.Errorf("delivery store requires db")
		}
		deliverStore = eventfabricrepo.NewDeliveryRepository(opts.DB)
	}

	var dlqStore dlqStore
	if opts.DLQ != nil {
		dlqStore = opts.DLQ
	} else if opts.DB != nil {
		dlqStore = eventfabricrepo.NewDlqRepository(opts.DB)
	}

	topics := opts.Topics
	if topics == nil {
		if opts.DB == nil {
			return nil, fmt.Errorf("topic store requires db")
		}
		topics = eventfabricrepo.NewTopicRepository(opts.DB)
	}

	aclRepo := opts.ACL
	if aclRepo == nil && opts.DB != nil {
		aclRepo = eventfabricrepo.NewAclRepository(opts.DB)
	}

	negotiator := opts.Negotiator
	if negotiator == nil {
		negotiator = &DefaultVersionNegotiator{}
	}

	metrics := opts.Metrics
	if metrics == nil {
		metrics = eventmetrics.NewNoop()
	}

	return &serviceImpl{
		db:                           opts.DB,
		envelopes:                    envStore,
		deliveries:                   deliverStore,
		dlq:                          dlqStore,
		topics:                       topics,
		acl:                          aclRepo,
		scheduler:                    opts.Scheduler,
		clock:                        clock,
		maxRetry:                     maxRetry,
		audit:                        opts.Audit,
		negotiator:                   negotiator,
		metrics:                      metrics,
		enableDatabaseFallbackLookup: opts.EnableDatabaseFallbackLookup,
	}, nil
}

func (s *serviceImpl) Publish(ctx context.Context, req PublishRequest) (err error) {
	start := s.clock().UTC()
	tenantKey, tenantErr := resolveTenantKey(req.TenantUUID)
	if tenantErr != nil {
		err = tenantErr
		return err
	}
	eventID := strings.TrimSpace(req.EventID)
	principal := strings.TrimSpace(req.Attributes["principal_id"])
	auditTopic := strings.TrimSpace(req.Topic)
	defer func() {
		if s.audit == nil {
			return
		}
		status := "SUCCESS"
		errMsg := ""
		if err != nil {
			status = "FAILED"
			errMsg = err.Error()
		}
		meta := map[string]string{
			"event_id": eventID,
		}
		if tenantKey != "" {
			meta["tenant_uuid"] = tenantKey
		}
		for k, v := range req.Attributes {
			if _, exists := meta[k]; !exists {
				meta[k] = v
			}
		}
		_ = s.audit.Write(ctx, eventaudit.Record{
			ID:           eventID,
			TenantID:     tenantKey,
			Topic:        auditTopic,
			PrincipalID:  principal,
			Action:       "publish",
			Status:       status,
			LatencyMs:    time.Since(start).Milliseconds(),
			TraceID:      req.TraceID,
			Metadata:     meta,
			HappenedAt:   start,
			ErrorMessage: errMsg,
		})
	}()

	if auditTopic == "" {
		err = fmt.Errorf("topic is required")
		return err
	}

	if eventID == "" {
		eventID = uuid.NewString()
	}

	topicTenant, namespace, name, parseErr := parseTopicName(auditTopic)
	if parseErr != nil {
		err = parseErr
		return err
	}

	if topicTenant != "" && !strings.EqualFold(topicTenant, tenantKey) && !isSharedTopicTenant(topicTenant) {
		err = shared.ErrTenantMismatch
		return err
	}

	topic, findErr := s.topics.FindByComposite(ctx, tenantKey, namespace, name)
	if findErr != nil {
		err = findErr
		return err
	}
	if topic == nil {
		err = fmt.Errorf("topic %s not found", req.Topic)
		return err
	}

	aclTenantKey := topic.TenantKey
	if aclTenantKey == "" {
		aclTenantKey = tenantKey
	}
	if !strings.EqualFold(topic.TenantKey, tenantKey) && !isSharedTopicTenant(topic.TenantKey) {
		err = shared.ErrTenantMismatch
		return err
	}
	auditTopic = topic.FullTopic

	if s.acl != nil && principal != "" {
		allowed, permErr := s.acl.HasPermission(ctx, aclTenantKey, topic.UUID, principal, string(aclPublish), s.clock().UTC())
		if permErr != nil {
			err = permErr
			return err
		}
		if !allowed {
			err = shared.ErrUnauthorized
			return err
		}
	}

	payload := req.Payload
	if len(payload) == 0 {
		payload = []byte("{}")
	}

	payloadJSON := datatypes.JSON(payload)
	var headersJSON datatypes.JSON
	if headersJSON, err = toJSON(req.Attributes); err != nil {
		return err
	}

	digest := sha256.Sum256(payload)
	maxRetry := topic.MaxRetry
	if maxRetry <= 0 {
		maxRetry = s.maxRetry
	}
	ackTimeout := topic.AckTimeoutSec
	if ackTimeout <= 0 {
		ackTimeout = int(shared.DefaultAckTimeout / time.Second)
	}

	envelope := &eventfabricmodel.EventEnvelope{
		TenantKey:      tenantKey,
		TopicUUID:      topic.UUID,
		EventID:        eventID,
		IdempotencyKey: req.IdempotencyKey,
		Version:        defaultVersion(req.Version),
		PayloadFormat:  defaultFormat(req.PayloadFormat),
		Payload:        payloadJSON,
		PayloadDigest:  hex.EncodeToString(digest[:]),
		Headers:        headersJSON,
		PublishedBy:    req.Attributes["principal_id"],
		PublishedAt:    s.clock().UTC(),
		Status:         shared.DeliveryStatusPending,
		RetryCount:     0,
		MaxRetry:       maxRetry,
		AckTimeoutSec:  ackTimeout,
		TraceID:        req.TraceID,
		LastError:      "",
	}

	storedEnvelope, existed, err := s.envelopes.UpsertByEventID(ctx, envelope)
	if err != nil {
		return err
	}
	if existed {
		return nil
	}

	subscribers, err := s.collectSubscribers(ctx, aclTenantKey, topic.UUID)
	if err != nil {
		return err
	}

	for _, subscriber := range subscribers {
		attempt := &eventfabricmodel.DeliveryAttempt{
			TenantKey:    tenantKey,
			EnvelopeUUID: storedEnvelope.UUID,
			EventID:      storedEnvelope.EventID,
			SubscriberID: subscriber,
			DeliveryNo:   1,
			Status:       shared.DeliveryStatusPending,
			TraceID:      storedEnvelope.TraceID,
		}

		attempt, err = s.deliveries.UpsertAttempt(ctx, attempt)
		if err != nil {
			return err
		}

		metadata := map[string]string{
			"attempt_uuid":    attempt.UUID.String(),
			"ack_timeout_sec": strconv.Itoa(ackTimeout),
		}

		if _, err := s.scheduler.Schedule(ctx, ScheduleOptions{
			TenantKey:    tenantKey,
			SubscriberID: subscriber,
			EventID:      storedEnvelope.EventID,
			EnvelopeUUID: storedEnvelope.UUID.String(),
			Attempt:      attempt.DeliveryNo,
			BaseDelay:    0,
			Metadata:     metadata,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *serviceImpl) Ack(ctx context.Context, deliveryID string, subscriberID string) (err error) {
	start := s.clock().UTC()
	var attempt *eventfabricmodel.DeliveryAttempt
	var envelope *eventfabricmodel.EventEnvelope
	var topicName string
	defer func() {
		if s.audit == nil {
			return
		}
		status := "SUCCESS"
		errMsg := ""
		if err != nil {
			status = "FAILED"
			errMsg = err.Error()
		}
		meta := map[string]string{
			"delivery_id": deliveryID,
		}
		if attempt != nil {
			meta["subscriber_id"] = attempt.SubscriberID
			meta["attempt_no"] = strconv.Itoa(attempt.DeliveryNo)
		}
		traceID := ""
		tenantKey := ""
		eventID := ""
		if envelope != nil {
			traceID = envelope.TraceID
			tenantKey = envelope.TenantKey
			eventID = envelope.EventID
			if tenantKey != "" {
				meta["tenant_uuid"] = tenantKey
				meta["tenant_uuid"] = tenantKey
			}
		}
		_ = s.audit.Write(ctx, eventaudit.Record{
			ID:           eventID,
			TenantID:     tenantKey,
			Topic:        topicName,
			PrincipalID:  subscriberID,
			Action:       "ack",
			Status:       status,
			LatencyMs:    time.Since(start).Milliseconds(),
			TraceID:      traceID,
			Metadata:     meta,
			HappenedAt:   start,
			ErrorMessage: errMsg,
		})
	}()

	attemptUUID, parseErr := uuid.Parse(deliveryID)
	if parseErr != nil {
		err = fmt.Errorf("invalid delivery id: %w", parseErr)
		return err
	}

	attempt, err = s.deliveries.FindByUUID(ctx, attemptUUID)
	if err != nil {
		return err
	}
	if attempt == nil {
		err = fmt.Errorf("delivery attempt %s not found", deliveryID)
		return err
	}
	if subscriberID != "" && !strings.EqualFold(subscriberID, attempt.SubscriberID) {
		err = fmt.Errorf("subscriber mismatch")
		return err
	}

	envelope, err = s.envelopes.FindByUUID(ctx, attempt.EnvelopeUUID)
	if err != nil {
		return err
	}
	if envelope == nil {
		err = fmt.Errorf("envelope missing for attempt %s", deliveryID)
		return err
	}
	if s.topics != nil {
		if topic, topicErr := s.topics.FindByUUID(ctx, envelope.TopicUUID); topicErr == nil && topic != nil {
			topicName = topic.FullTopic
		}
	}

	now := s.clock().UTC()
	var latency int
	if attempt.LastAttemptAt != nil {
		latency = int(now.Sub(*attempt.LastAttemptAt).Milliseconds())
	}

	if err := s.deliveries.UpdateStatus(ctx, attempt.UUID, map[string]interface{}{
		"status":          shared.DeliveryStatusSucceeded,
		"acked_at":        now,
		"latency_ms":      latency,
		"last_error_code": nil,
		"nack_reason":     "",
	}); err != nil {
		return err
	}

	active, err := s.deliveries.CountActiveAttempts(ctx, attempt.EnvelopeUUID)
	if err != nil {
		return err
	}
	status := shared.DeliveryStatusSucceeded
	if active > 0 {
		status = shared.DeliveryStatusDelivering
	}
	if err := s.envelopes.UpdateStatus(ctx, attempt.EnvelopeUUID, map[string]interface{}{
		"status":      status,
		"retry_count": attempt.DeliveryNo - 1,
	}); err != nil {
		return err
	}

	ackTimeout := time.Duration(envelope.AckTimeoutSec) * time.Second
	latencyDuration := time.Duration(latency) * time.Millisecond
	s.metrics.ObserveDelivery(ctx, true, latencyDuration)
	return s.scheduler.ReleaseLease(ctx, eventbus.DeliveryLease{
		LeaseID:       attempt.UUID.String(),
		TenantKey:     attempt.TenantKey,
		SubscriberID:  attempt.SubscriberID,
		AckTimeout:    ackTimeout,
		MaxConcurrent: 1,
	})
}

func (s *serviceImpl) Nack(ctx context.Context, deliveryID string, subscriberID string, reason string) (plan RetryPlan, err error) {
	start := s.clock().UTC()
	var attempt *eventfabricmodel.DeliveryAttempt
	var envelope *eventfabricmodel.EventEnvelope
	var topicName string
	defer func() {
		if s.audit == nil {
			return
		}
		status := "SUCCESS"
		errMsg := ""
		if err != nil {
			status = "FAILED"
			errMsg = err.Error()
		}
		meta := map[string]string{
			"delivery_id": deliveryID,
			"reason":      reason,
		}
		if attempt != nil {
			meta["subscriber_id"] = attempt.SubscriberID
			meta["attempt_no"] = strconv.Itoa(attempt.DeliveryNo)
		}
		if plan.NextDelay > 0 {
			meta["next_delay_ms"] = strconv.FormatInt(int64(plan.NextDelay/time.Millisecond), 10)
		}
		traceID := ""
		tenantKey := ""
		eventID := ""
		if envelope != nil {
			traceID = envelope.TraceID
			tenantKey = envelope.TenantKey
			eventID = envelope.EventID
			if tenantKey != "" {
				meta["tenant_uuid"] = tenantKey
				meta["tenant_uuid"] = tenantKey
			}
		}
		_ = s.audit.Write(ctx, eventaudit.Record{
			ID:           eventID,
			TenantID:     tenantKey,
			Topic:        topicName,
			PrincipalID:  subscriberID,
			Action:       "nack",
			Status:       status,
			LatencyMs:    time.Since(start).Milliseconds(),
			TraceID:      traceID,
			Metadata:     meta,
			HappenedAt:   start,
			ErrorMessage: errMsg,
		})
	}()

	attemptUUID, parseErr := uuid.Parse(deliveryID)
	if parseErr != nil {
		err = fmt.Errorf("invalid delivery id: %w", parseErr)
		return plan, err
	}

	attempt, err = s.deliveries.FindByUUID(ctx, attemptUUID)
	if err != nil {
		return plan, err
	}
	if attempt == nil {
		err = fmt.Errorf("delivery attempt %s not found", deliveryID)
		return plan, err
	}
	if subscriberID != "" && !strings.EqualFold(subscriberID, attempt.SubscriberID) {
		err = fmt.Errorf("subscriber mismatch")
		return plan, err
	}

	envelope, err = s.envelopes.FindByUUID(ctx, attempt.EnvelopeUUID)
	if err != nil {
		return plan, err
	}
	if envelope == nil {
		err = fmt.Errorf("envelope missing for attempt %s", deliveryID)
		return plan, err
	}
	if s.topics != nil {
		if topic, topicErr := s.topics.FindByUUID(ctx, envelope.TopicUUID); topicErr == nil && topic != nil {
			topicName = topic.FullTopic
		}
	}

	nextAttempt := attempt.DeliveryNo + 1
	maxAttempts := envelope.MaxRetry
	if maxAttempts <= 0 {
		maxAttempts = s.maxRetry
	}

	var previousAttemptAt *time.Time
	if attempt.LastAttemptAt != nil {
		value := *attempt.LastAttemptAt
		previousAttemptAt = &value
	}

	now := s.clock().UTC()
	if err := s.deliveries.UpdateStatus(ctx, attempt.UUID, map[string]interface{}{
		"status":          shared.DeliveryStatusFailed,
		"last_error_code": "nack",
		"nack_reason":     reason,
		"last_attempt_at": now,
	}); err != nil {
		return plan, err
	}

	ackTimeout := time.Duration(envelope.AckTimeoutSec) * time.Second
	_ = s.scheduler.ReleaseLease(ctx, eventbus.DeliveryLease{
		LeaseID:       attempt.UUID.String(),
		TenantKey:     attempt.TenantKey,
		SubscriberID:  attempt.SubscriberID,
		AckTimeout:    ackTimeout,
		MaxConcurrent: 1,
	})

	if nextAttempt > maxAttempts {
		if s.dlq != nil {
			if err := s.pushToDLQ(ctx, envelope, attempt, reason); err != nil {
				return plan, err
			}
		}
		if err := s.envelopes.UpdateStatus(ctx, attempt.EnvelopeUUID, map[string]interface{}{
			"status":     shared.DeliveryStatusFailed,
			"last_error": reason,
		}); err != nil {
			return plan, err
		}
		latency := time.Duration(0)
		if previousAttemptAt != nil {
			latency = now.Sub(*previousAttemptAt)
		}
		s.metrics.ObserveDelivery(ctx, false, latency)
		plan = RetryPlan{
			MaxAttempts:       int32(maxAttempts),
			RemainingAttempts: 0,
			NextDelay:         0,
			Strategy:          "exponential-jitter",
		}
		err = shared.ErrRetryExhausted
		return plan, err
	}

	newAttempt := &eventfabricmodel.DeliveryAttempt{
		TenantKey:    attempt.TenantKey,
		EnvelopeUUID: attempt.EnvelopeUUID,
		EventID:      attempt.EventID,
		SubscriberID: attempt.SubscriberID,
		DeliveryNo:   nextAttempt,
		Status:       shared.DeliveryStatusPending,
		TraceID:      attempt.TraceID,
	}

	newAttempt, err = s.deliveries.UpsertAttempt(ctx, newAttempt)
	if err != nil {
		return plan, err
	}

	metadata := map[string]string{
		"attempt_uuid":    newAttempt.UUID.String(),
		"ack_timeout_sec": strconv.Itoa(envelope.AckTimeoutSec),
	}

	item, err := s.scheduler.Schedule(ctx, ScheduleOptions{
		TenantKey:    attempt.TenantKey,
		SubscriberID: attempt.SubscriberID,
		EventID:      attempt.EventID,
		EnvelopeUUID: attempt.EnvelopeUUID.String(),
		Attempt:      nextAttempt,
		BaseDelay:    shared.DefaultAckTimeout / 6,
		Metadata:     metadata,
	})
	if err != nil {
		return plan, err
	}
	s.metrics.ObserveRetry(ctx, item.Backoff)

	if err := s.envelopes.UpdateStatus(ctx, attempt.EnvelopeUUID, map[string]interface{}{
		"status":      shared.DeliveryStatusPending,
		"retry_count": nextAttempt - 1,
		"last_error":  reason,
	}); err != nil {
		return plan, err
	}

	remaining := maxAttempts - nextAttempt + 1
	if remaining < 0 {
		remaining = 0
	}
	plan = RetryPlan{
		MaxAttempts:       int32(maxAttempts),
		RemainingAttempts: int32(remaining),
		NextDelay:         item.Backoff,
		Strategy:          "exponential-jitter",
	}
	return plan, nil
}

func (s *serviceImpl) PollRetry(ctx context.Context, limit int) (map[string][]DeliveryAttempt, error) {
	tenantKey := shared.TenantFromContext(ctx)
	if tenantKey == "" {
		return nil, shared.ErrTenantMismatch
	}
	if limit <= 0 {
		limit = 50
	}
	subscriberFilter := strings.TrimSpace(shared.SubscriberFromContext(ctx))
	topicFilter := shared.TopicFilterFromContext(ctx)

	items, err := s.scheduler.PopDue(ctx, tenantKey, limit)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]DeliveryAttempt)
	now := s.clock().UTC()

	for _, item := range items {
		attemptUUID, err := uuid.Parse(item.Metadata["attempt_uuid"])
		if err != nil {
			// fallback
			attemptUUID = uuid.Nil
		}

		var attempt *eventfabricmodel.DeliveryAttempt
		if attemptUUID != uuid.Nil {
			attempt, err = s.deliveries.FindByUUID(ctx, attemptUUID)
			if err != nil {
				return nil, err
			}
		}
		if attempt == nil {
			if !s.enableDatabaseFallbackLookup {
				continue
			}
			// fallback by envelope/subscriber
			envelopeUUID, err := uuid.Parse(item.EnvelopeUUID)
			if err != nil {
				continue
			}
			attempt, err = s.deliveries.FindByEnvelopeAndSubscriber(ctx, envelopeUUID, item.SubscriberID)
			if err != nil || attempt == nil {
				continue
			}
		}

		envelope, err := s.envelopes.FindByUUID(ctx, attempt.EnvelopeUUID)
		if err != nil || envelope == nil {
			continue
		}

		if subscriberFilter != "" && !strings.EqualFold(attempt.SubscriberID, subscriberFilter) {
			s.rescheduleAttempt(ctx, attempt, envelope)
			continue
		}

		leaseGranted, err := s.scheduler.AcquireLease(ctx, eventbus.DeliveryLease{
			LeaseID:       attempt.UUID.String(),
			TenantKey:     attempt.TenantKey,
			SubscriberID:  attempt.SubscriberID,
			AckTimeout:    time.Duration(envelope.AckTimeoutSec) * time.Second,
			MaxConcurrent: 1,
		})
		if err != nil {
			return nil, err
		}
		if !leaseGranted {
			// 重新排队，避免并发超限。
			item, err := s.scheduler.Schedule(ctx, ScheduleOptions{
				TenantKey:    attempt.TenantKey,
				SubscriberID: attempt.SubscriberID,
				EventID:      attempt.EventID,
				EnvelopeUUID: attempt.EnvelopeUUID.String(),
				Attempt:      attempt.DeliveryNo,
				BaseDelay:    200 * time.Millisecond,
				Metadata: map[string]string{
					"attempt_uuid":    attempt.UUID.String(),
					"ack_timeout_sec": strconv.Itoa(envelope.AckTimeoutSec),
				},
			})
			if err == nil {
				s.metrics.ObserveRetry(ctx, item.Backoff)
			}
			continue
		}

		if err := s.deliveries.UpdateStatus(ctx, attempt.UUID, map[string]interface{}{
			"status":          shared.DeliveryStatusDelivering,
			"last_attempt_at": now,
		}); err != nil {
			return nil, err
		}

		topicFullName := fmt.Sprintf("%s", envelope.TenantKey)
		var topicRecord *eventfabricmodel.TopicDefinition
		if s.topics != nil {
			if topic, err := s.topics.FindByUUID(ctx, envelope.TopicUUID); err == nil && topic != nil {
				topicRecord = topic
				if topic.FullTopic != "" {
					topicFullName = topic.FullTopic
				}
			}
		}

		if topicFilter != nil {
			if _, ok := topicFilter[strings.ToLower(strings.TrimSpace(topicFullName))]; !ok {
				s.rescheduleAttempt(ctx, attempt, envelope)
				_ = s.deliveries.UpdateStatus(ctx, attempt.UUID, map[string]interface{}{
					"status": shared.DeliveryStatusPending,
				})
				_ = s.scheduler.ReleaseLease(ctx, eventbus.DeliveryLease{
					LeaseID:       attempt.UUID.String(),
					TenantKey:     attempt.TenantKey,
					SubscriberID:  attempt.SubscriberID,
					AckTimeout:    time.Duration(envelope.AckTimeoutSec) * time.Second,
					MaxConcurrent: 1,
				})
				continue
			}
		}

		compatibilityMode := shared.CompatibilityModeFromContext(ctx)
		if compatibilityMode == "" && topicRecord != nil && strings.TrimSpace(topicRecord.VersioningMode) != "" {
			compatibilityMode = strings.ToLower(strings.TrimSpace(topicRecord.VersioningMode))
		}
		if compatibilityMode == "" {
			compatibilityMode = string(CompatibilityModeAny)
		}

		supportedVersions := shared.AcceptedVersionsFromContext(ctx)
		negotiation := s.negotiator.Negotiate(CompatibilityMode(compatibilityMode), envelope.Version, supportedVersions)
		if !negotiation.Compatible {
			s.handleVersionIncompatible(ctx, attempt, envelope, negotiation.Reason)
			continue
		}

		headers, _ := fromJSONMap(envelope.Headers)
		if headers == nil {
			headers = map[string]string{}
		}
		headers["version_mode"] = string(negotiation.Mode)
		headers["version_compatible"] = "true"
		if negotiation.SelectedVersion != "" {
			headers["version_selected"] = negotiation.SelectedVersion
		}

		maxAttempts := envelope.MaxRetry
		if maxAttempts <= 0 {
			maxAttempts = s.maxRetry
		}
		remaining := maxAttempts - attempt.DeliveryNo + 1
		if remaining < 0 {
			remaining = 0
		}

		result[item.EventID] = append(result[item.EventID], DeliveryAttempt{
			AttemptNumber: int32(attempt.DeliveryNo),
			SubscriberID:  attempt.SubscriberID,
			StartedAt:     now,
			Status:        shared.DeliveryStatusDelivering,
			EventID:       envelope.EventID,
			DeliveryUUID:  attempt.UUID.String(),
			EnvelopeUUID:  envelope.UUID.String(),
			Payload:       []byte(envelope.Payload),
			Headers:       headers,
			Version:       envelope.Version,
			PayloadFormat: envelope.PayloadFormat,
			TraceID:       envelope.TraceID,
			TopicFullName: topicFullName,
			AckTimeout:    time.Duration(envelope.AckTimeoutSec) * time.Second,
			MaxAttempts:   int32(maxAttempts),
			Remaining:     int32(remaining),
		})
	}
	return result, nil
}

func (s *serviceImpl) collectSubscribers(ctx context.Context, tenantKey string, topicUUID uuid.UUID) ([]string, error) {
	if s.acl == nil {
		return []string{}, nil
	}
	records, err := s.acl.ListByTopic(ctx, tenantKey, topicUUID)
	if err != nil {
		return nil, err
	}
	now := s.clock().UTC()
	var subscribers []string
	for _, binding := range records {
		if binding.Action != string(aclSubscribe) {
			continue
		}
		if binding.Status != 1 {
			continue
		}
		if binding.ExpiresAt != nil && binding.ExpiresAt.Before(now) {
			continue
		}
		subscribers = append(subscribers, binding.PrincipalID)
	}
	return subscribers, nil
}

func (s *serviceImpl) pushToDLQ(ctx context.Context, envelope *eventfabricmodel.EventEnvelope, attempt *eventfabricmodel.DeliveryAttempt, reason string) error {
	if s.dlq == nil {
		return nil
	}
	_, err := s.dlq.Create(ctx, &eventfabricmodel.DlqMessage{
		TenantKey:       envelope.TenantKey,
		TopicUUID:       envelope.TopicUUID,
		EnvelopeUUID:    envelope.UUID,
		EventID:         envelope.EventID,
		FailureStage:    "delivery",
		LastErrorCode:   "nack",
		LastErrorMsg:    reason,
		PayloadSnapshot: envelope.Payload,
		Headers:         envelope.Headers,
		Status:          "queued",
		TraceID:         envelope.TraceID,
	})
	if err == nil {
		s.metrics.ObserveDLQChange(ctx, 1)
	}
	return err
}

func (s *serviceImpl) handleVersionIncompatible(ctx context.Context, attempt *eventfabricmodel.DeliveryAttempt, envelope *eventfabricmodel.EventEnvelope, reason string) {
	if attempt == nil || envelope == nil {
		return
	}
	now := s.clock().UTC()
	_ = s.deliveries.UpdateStatus(ctx, attempt.UUID, map[string]interface{}{
		"status":          shared.DeliveryStatusFailed,
		"last_error_code": "version_incompatible",
		"nack_reason":     reason,
		"last_attempt_at": now,
	})
	if s.dlq != nil {
		_ = s.pushToDLQ(ctx, envelope, attempt, reason)
	}
	latency := time.Duration(0)
	if attempt.LastAttemptAt != nil {
		latency = now.Sub(*attempt.LastAttemptAt)
	}
	s.metrics.ObserveDelivery(ctx, false, latency)
	_ = s.envelopes.UpdateStatus(ctx, envelope.UUID, map[string]interface{}{
		"status":     shared.DeliveryStatusFailed,
		"last_error": reason,
	})
	_ = s.scheduler.ReleaseLease(ctx, eventbus.DeliveryLease{
		LeaseID:       attempt.UUID.String(),
		TenantKey:     attempt.TenantKey,
		SubscriberID:  attempt.SubscriberID,
		AckTimeout:    time.Duration(envelope.AckTimeoutSec) * time.Second,
		MaxConcurrent: 1,
	})
}

func (s *serviceImpl) rescheduleAttempt(ctx context.Context, attempt *eventfabricmodel.DeliveryAttempt, envelope *eventfabricmodel.EventEnvelope) {
	if s.scheduler == nil || attempt == nil || envelope == nil {
		return
	}
	metadata := map[string]string{
		"attempt_uuid":    attempt.UUID.String(),
		"ack_timeout_sec": strconv.Itoa(envelope.AckTimeoutSec),
	}
	item, err := s.scheduler.Schedule(ctx, ScheduleOptions{
		TenantKey:    attempt.TenantKey,
		SubscriberID: attempt.SubscriberID,
		EventID:      attempt.EventID,
		EnvelopeUUID: attempt.EnvelopeUUID.String(),
		Attempt:      attempt.DeliveryNo,
		BaseDelay:    0,
		Metadata:     metadata,
	})
	if err == nil {
		s.metrics.ObserveRetry(ctx, item.Backoff)
	}
}

func resolveTenantKey(value string) (string, error) {
	if key := strings.TrimSpace(value); key != "" {
		return key, nil
	}
	return "", fmt.Errorf("tenant_uuid is required")
}

func parseTopicName(topic string) (tenant, namespace, name string, err error) {
	parts := strings.Split(strings.TrimSpace(topic), ".")
	if len(parts) < 2 {
		return "", "", "", fmt.Errorf("invalid topic format: %s", topic)
	}

	first := strings.TrimSpace(parts[0])
	if parsed, parseErr := uuid.Parse(first); parseErr == nil && parsed != uuid.Nil && len(parts) >= 3 {
		tenant = first
		namespace = strings.Join(parts[1:len(parts)-1], ".")
		name = parts[len(parts)-1]
		return tenant, namespace, name, nil
	}

	namespace = strings.Join(parts[:len(parts)-1], ".")
	name = parts[len(parts)-1]
	return "", namespace, name, nil
}

func isSharedTopicTenant(tenantKey string) bool {
	key := strings.ToLower(strings.TrimSpace(tenantKey))
	return key == "global" || key == "system"
}

func defaultVersion(version string) string {
	if strings.TrimSpace(version) == "" {
		return "v1"
	}
	return version
}

func defaultFormat(format string) string {
	if strings.TrimSpace(format) == "" {
		return "json"
	}
	return strings.ToLower(format)
}

func toJSON(source map[string]string) (datatypes.JSON, error) {
	if len(source) == 0 {
		return datatypes.JSON([]byte(`{}`)), nil
	}
	encoded, err := json.Marshal(source)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(encoded), nil
}

func fromJSONMap(data datatypes.JSON) (map[string]string, error) {
	if len(data) == 0 {
		return map[string]string{}, nil
	}
	var result map[string]string
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

type aclAction string

const (
	aclPublish   aclAction = "publish"
	aclSubscribe aclAction = "subscribe"
)
