package eventfabric

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/delivery"
	eventmetrics "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/metrics"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/shared"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
)

type benchEnv struct {
	service    delivery.Service
	queue      *memoryQueue
	tenant     string
	topic      *eventfabricmodel.TopicDefinition
	subscriber string
}

func newBenchEnv(b *testing.B) *benchEnv {
	b.Helper()

	queue := newMemoryQueue()
	scheduler := delivery.NewBackoffScheduler(queue)
	scheduler.WithClock(func() time.Time { return time.Now().UTC() })

	envelopes := newMemoryEnvelopeStore()
	deliveries := newMemoryDeliveryStore()
	dlq := newMemoryDLQStore()
	topics := newMemoryTopicStore()
	acls := newMemoryACLStore()

	tenantKey := "tenant-bench"
	topic := topics.addTopic(tenantKey, "corex.workflow", "approved", 30, 5)
	acls.allowSubscriber(topic.UUID, "svc-bench-consumer")

	svc, err := delivery.NewService(delivery.Options{
		Envelopes:  envelopes,
		Deliveries: deliveries,
		DLQ:        dlq,
		Topics:     topics,
		ACL:        acls,
		Scheduler:  scheduler,
		Clock:      time.Now,
		MaxRetry:   5,
		Metrics:    eventmetrics.NewNoop(),
	})
	if err != nil {
		b.Fatalf("build delivery service: %v", err)
	}

	return &benchEnv{
		service:    svc,
		queue:      queue,
		tenant:     tenantKey,
		topic:      topic,
		subscriber: "svc-bench-consumer",
	}
}

func (e *benchEnv) pollOnce(ctx context.Context) (delivery.DeliveryAttempt, bool, error) {
	records, err := e.service.PollRetry(ctx, 1)
	if err != nil {
		return delivery.DeliveryAttempt{}, false, err
	}
	for _, attempts := range records {
		if len(attempts) == 0 {
			continue
		}
		return attempts[0], true, nil
	}
	return delivery.DeliveryAttempt{}, false, nil
}

// --- Memory implementations for delivery service dependencies ---

type memoryEnvelopeStore struct {
	mu        sync.RWMutex
	byEventID map[string]*eventfabricmodel.EventEnvelope
	byUUID    map[uuid.UUID]*eventfabricmodel.EventEnvelope
}

func newMemoryEnvelopeStore() *memoryEnvelopeStore {
	return &memoryEnvelopeStore{
		byEventID: map[string]*eventfabricmodel.EventEnvelope{},
		byUUID:    map[uuid.UUID]*eventfabricmodel.EventEnvelope{},
	}
}

func (s *memoryEnvelopeStore) key(tenant, eventID string) string {
	return fmt.Sprintf("%s|%s", tenant, eventID)
}

func (s *memoryEnvelopeStore) UpsertByEventID(ctx context.Context, envelope *eventfabricmodel.EventEnvelope) (*eventfabricmodel.EventEnvelope, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := s.key(envelope.TenantKey, envelope.EventID)
	if existing, ok := s.byEventID[key]; ok {
		return cloneEnvelope(existing), true, nil
	}
	copy := cloneEnvelope(envelope)
	if copy.UUID == uuid.Nil {
		copy.UUID = uuid.New()
	}
	s.byEventID[key] = copy
	s.byUUID[copy.UUID] = copy
	return cloneEnvelope(copy), false, nil
}

func (s *memoryEnvelopeStore) FindByEventID(ctx context.Context, tenantKey, eventID string) (*eventfabricmodel.EventEnvelope, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if env, ok := s.byEventID[s.key(tenantKey, eventID)]; ok {
		return cloneEnvelope(env), nil
	}
	return nil, nil
}

func (s *memoryEnvelopeStore) FindByUUID(ctx context.Context, id uuid.UUID) (*eventfabricmodel.EventEnvelope, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if env, ok := s.byUUID[id]; ok {
		return cloneEnvelope(env), nil
	}
	return nil, nil
}

func (s *memoryEnvelopeStore) UpdateStatus(ctx context.Context, envelopeUUID uuid.UUID, updates map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	env, ok := s.byUUID[envelopeUUID]
	if !ok {
		return fmt.Errorf("envelope %s not found", envelopeUUID)
	}
	if status, ok := updates["status"].(string); ok {
		env.Status = status
	}
	if retryCount, ok := updates["retry_count"].(int); ok {
		env.RetryCount = retryCount
	}
	if retryCount, ok := updates["retry_count"].(int32); ok {
		env.RetryCount = int(retryCount)
	}
	if errMsg, ok := updates["last_error"].(string); ok {
		env.LastError = errMsg
	}
	if ackTimeout, ok := updates["ack_timeout"].(time.Duration); ok {
		env.AckTimeoutSec = int(ackTimeout / time.Second)
	}
	return nil
}

type memoryDeliveryStore struct {
	mu     sync.RWMutex
	byUUID map[uuid.UUID]*eventfabricmodel.DeliveryAttempt
}

func newMemoryDeliveryStore() *memoryDeliveryStore {
	return &memoryDeliveryStore{
		byUUID: map[uuid.UUID]*eventfabricmodel.DeliveryAttempt{},
	}
}

func (s *memoryDeliveryStore) UpsertAttempt(ctx context.Context, attempt *eventfabricmodel.DeliveryAttempt) (*eventfabricmodel.DeliveryAttempt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := cloneAttempt(attempt)
	if copy.UUID == uuid.Nil {
		copy.UUID = uuid.New()
	}
	s.byUUID[copy.UUID] = copy
	return cloneAttempt(copy), nil
}

func (s *memoryDeliveryStore) FindByEnvelopeAndSubscriber(ctx context.Context, envelope uuid.UUID, subscriberID string) (*eventfabricmodel.DeliveryAttempt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, attempt := range s.byUUID {
		if attempt.EnvelopeUUID == envelope && attempt.SubscriberID == subscriberID {
			return cloneAttempt(attempt), nil
		}
	}
	return nil, nil
}

func (s *memoryDeliveryStore) FindByUUID(ctx context.Context, id uuid.UUID) (*eventfabricmodel.DeliveryAttempt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if attempt, ok := s.byUUID[id]; ok {
		return cloneAttempt(attempt), nil
	}
	return nil, nil
}

func (s *memoryDeliveryStore) UpdateStatus(ctx context.Context, attemptUUID uuid.UUID, updates map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	attempt, ok := s.byUUID[attemptUUID]
	if !ok {
		return fmt.Errorf("attempt %s not found", attemptUUID)
	}
	if status, ok := updates["status"].(string); ok {
		attempt.Status = status
	}
	if errCode, ok := updates["last_error_code"].(string); ok {
		attempt.LastErrorCode = errCode
	}
	if nackReason, ok := updates["nack_reason"].(string); ok {
		attempt.NackReason = nackReason
	}
	if lastAttemptAt, ok := updates["last_attempt_at"].(time.Time); ok {
		attempt.LastAttemptAt = &lastAttemptAt
	}
	if ackedAt, ok := updates["acked_at"].(time.Time); ok {
		attempt.AckedAt = &ackedAt
	}
	if latency, ok := updates["latency_ms"].(int); ok {
		attempt.LatencyMs = latency
	}
	return nil
}

func (s *memoryDeliveryStore) CountActiveAttempts(ctx context.Context, envelope uuid.UUID) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var count int64
	for _, attempt := range s.byUUID {
		if attempt.EnvelopeUUID != envelope {
			continue
		}
		if attempt.Status == shared.DeliveryStatusSucceeded || attempt.Status == shared.DeliveryStatusFailed {
			continue
		}
		count++
	}
	return count, nil
}

type memoryDLQStore struct {
	mu     sync.Mutex
	items  []*eventfabricmodel.DlqMessage
	lastID atomic.Uint64
}

func newMemoryDLQStore() *memoryDLQStore {
	return &memoryDLQStore{}
}

func (s *memoryDLQStore) Create(ctx context.Context, message *eventfabricmodel.DlqMessage) (*eventfabricmodel.DlqMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *message
	copy.UUID = uuid.New()
	s.items = append(s.items, &copy)
	return &copy, nil
}

type memoryTopicStore struct {
	mu     sync.RWMutex
	topics map[string]*eventfabricmodel.TopicDefinition
}

func newMemoryTopicStore() *memoryTopicStore {
	return &memoryTopicStore{
		topics: map[string]*eventfabricmodel.TopicDefinition{},
	}
}

func (s *memoryTopicStore) composite(tenant, namespace, name string) string {
	return fmt.Sprintf("%s|%s|%s", tenant, namespace, name)
}

func (s *memoryTopicStore) addTopic(tenant, namespace, name string, ackTimeout, maxRetry int) *eventfabricmodel.TopicDefinition {
	s.mu.Lock()
	defer s.mu.Unlock()
	topic := &eventfabricmodel.TopicDefinition{
		UUID:          uuid.New(),
		TenantKey:     tenant,
		TenantID:      1,
		Namespace:     namespace,
		Name:          name,
		FullTopic:     fmt.Sprintf("%s.%s.%s", tenant, namespace, name),
		MaxRetry:      maxRetry,
		AckTimeoutSec: ackTimeout,
		Status:        1,
	}
	s.topics[s.composite(tenant, namespace, name)] = topic
	s.topics[topic.UUID.String()] = topic
	return topic
}

func (s *memoryTopicStore) FindByComposite(ctx context.Context, tenantKey, namespace, name string) (*eventfabricmodel.TopicDefinition, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if topic, ok := s.topics[s.composite(tenantKey, namespace, name)]; ok {
		return cloneTopic(topic), nil
	}
	return nil, nil
}

func (s *memoryTopicStore) FindByUUID(ctx context.Context, id uuid.UUID) (*eventfabricmodel.TopicDefinition, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if topic, ok := s.topics[id.String()]; ok {
		return cloneTopic(topic), nil
	}
	return nil, nil
}

type memoryACLStore struct {
	mu          sync.RWMutex
	subscribers map[uuid.UUID][]*eventfabricmodel.AclBinding
}

func newMemoryACLStore() *memoryACLStore {
	return &memoryACLStore{
		subscribers: map[uuid.UUID][]*eventfabricmodel.AclBinding{},
	}
}

func (s *memoryACLStore) allowSubscriber(topic uuid.UUID, subscriber string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscribers[topic] = append(s.subscribers[topic], &eventfabricmodel.AclBinding{
		UUID:          uuid.New(),
		TenantKey:     "tenant-bench",
		TopicUUID:     topic,
		PrincipalID:   subscriber,
		PrincipalType: "service",
		Action:        "subscribe",
		Status:        1,
	})
}

func (s *memoryACLStore) HasPermission(ctx context.Context, tenantKey string, topic uuid.UUID, principalID string, action string, now time.Time) (bool, error) {
	return true, nil
}

func (s *memoryACLStore) ListByTopic(ctx context.Context, tenantKey string, topic uuid.UUID) ([]*eventfabricmodel.AclBinding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := s.subscribers[topic]
	result := make([]*eventfabricmodel.AclBinding, 0, len(list))
	for _, item := range list {
		result = append(result, cloneBinding(item))
	}
	return result, nil
}

type memoryQueue struct {
	mu     sync.Mutex
	items  []eventbus.RetryItem
	leases map[string]int
}

func newMemoryQueue() *memoryQueue {
	return &memoryQueue{
		items:  []eventbus.RetryItem{},
		leases: map[string]int{},
	}
}

func (q *memoryQueue) ScheduleRetry(ctx context.Context, item eventbus.RetryItem) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if item.ExecuteAt.IsZero() {
		item.ExecuteAt = time.Now().UTC()
	}
	q.items = append(q.items, item)
	return nil
}

func (q *memoryQueue) PopDueRetries(ctx context.Context, tenantKey string, now time.Time, limit int) ([]eventbus.RetryItem, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if limit <= 0 {
		limit = 64
	}
	var ready []eventbus.RetryItem
	var remaining []eventbus.RetryItem
	for _, item := range q.items {
		if len(ready) >= limit {
			remaining = append(remaining, item)
			continue
		}
		if item.TenantKey != tenantKey {
			remaining = append(remaining, item)
			continue
		}
		if item.ExecuteAt.After(now) {
			remaining = append(remaining, item)
			continue
		}
		ready = append(ready, item)
	}
	q.items = remaining
	return ready, nil
}

func (q *memoryQueue) RemoveRetry(ctx context.Context, item eventbus.RetryItem) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	next := q.items[:0]
	for _, existing := range q.items {
		if existing.EventID == item.EventID && existing.EnvelopeUUID == item.EnvelopeUUID && existing.SubscriberID == item.SubscriberID {
			continue
		}
		next = append(next, existing)
	}
	q.items = next
	return nil
}

func (q *memoryQueue) AcquireLease(ctx context.Context, lease eventbus.DeliveryLease) (bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	key := fmt.Sprintf("%s|%s", lease.TenantKey, lease.SubscriberID)
	q.leases[key]++
	return true, nil
}

func (q *memoryQueue) ReleaseLease(ctx context.Context, lease eventbus.DeliveryLease) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	key := fmt.Sprintf("%s|%s", lease.TenantKey, lease.SubscriberID)
	if current := q.leases[key]; current <= 1 {
		delete(q.leases, key)
	} else {
		q.leases[key] = current - 1
	}
	return nil
}

// --- Helpers to avoid sharing mutable state across operations ---

func cloneEnvelope(src *eventfabricmodel.EventEnvelope) *eventfabricmodel.EventEnvelope {
	if src == nil {
		return nil
	}
	copy := *src
	return &copy
}

func cloneAttempt(src *eventfabricmodel.DeliveryAttempt) *eventfabricmodel.DeliveryAttempt {
	if src == nil {
		return nil
	}
	copy := *src
	return &copy
}

func cloneTopic(src *eventfabricmodel.TopicDefinition) *eventfabricmodel.TopicDefinition {
	if src == nil {
		return nil
	}
	copy := *src
	return &copy
}

func cloneBinding(src *eventfabricmodel.AclBinding) *eventfabricmodel.AclBinding {
	if src == nil {
		return nil
	}
	copy := *src
	return &copy
}
