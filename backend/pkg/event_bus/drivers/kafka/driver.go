package kafka

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	eventbus "github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

// ConsumerGroupSession 描述消费组会话上下文。
type ConsumerGroupSession struct {
	GroupID    string
	MemberID   string
	Generation int64
	Assigned   []int32
}

// OffsetCommit 表示一次 offset 提交。
type OffsetCommit struct {
	Topic     string
	Partition int32
	Offset    int64
	Metadata  string
}

// RebalanceHandler 定义 rebalance 生命周期回调。
type RebalanceHandler interface {
	OnAssigned(ctx context.Context, session ConsumerGroupSession)
	OnRevoked(ctx context.Context, session ConsumerGroupSession)
}

// RebalanceHandlerFunc 用于快速注入回调。
type RebalanceHandlerFunc struct {
	Assigned func(context.Context, ConsumerGroupSession)
	Revoked  func(context.Context, ConsumerGroupSession)
}

func (f RebalanceHandlerFunc) OnAssigned(ctx context.Context, session ConsumerGroupSession) {
	if f.Assigned != nil {
		f.Assigned(ctx, session)
	}
}

func (f RebalanceHandlerFunc) OnRevoked(ctx context.Context, session ConsumerGroupSession) {
	if f.Revoked != nil {
		f.Revoked(ctx, session)
	}
}

// DriverOptions 描述 Kafka 任务驱动参数（当前为契约适配层，具体 broker I/O 在后续任务增强）。
type DriverOptions struct {
	Brokers        []string
	TopicPrefix    string
	ConsumerGroup  string
	PollTimeout    time.Duration
	FallbackDriver eventbus.TaskDriver
	Rebalance      RebalanceHandler
}

type driver struct {
	brokers       []string
	topicPrefix   string
	consumerGroup string
	pollTimeout   time.Duration
	fallback      eventbus.TaskDriver
	rebalance     RebalanceHandler

	mu             sync.RWMutex
	runningSession *ConsumerGroupSession
	commits        []OffsetCommit
}

// NewDriver 创建 Kafka TaskDriver 适配层。
func NewDriver(opts DriverOptions) eventbus.TaskDriver {
	topicPrefix := strings.TrimSpace(opts.TopicPrefix)
	if topicPrefix == "" {
		topicPrefix = "event_fabric.task"
	}
	consumerGroup := strings.TrimSpace(opts.ConsumerGroup)
	if consumerGroup == "" {
		consumerGroup = "powerx.event_fabric"
	}
	pollTimeout := opts.PollTimeout
	if pollTimeout <= 0 {
		pollTimeout = time.Second
	}
	brokers := normalizeBrokers(opts.Brokers)
	if opts.Rebalance == nil {
		opts.Rebalance = RebalanceHandlerFunc{}
	}

	return &driver{
		brokers:       brokers,
		topicPrefix:   topicPrefix,
		consumerGroup: consumerGroup,
		pollTimeout:   pollTimeout,
		fallback:      opts.FallbackDriver,
		rebalance:     opts.Rebalance,
	}
}

func (d *driver) Type() eventbus.QueueDriverType {
	return eventbus.QueueDriverKafka
}

func (d *driver) Capability() eventbus.QueueDriverCapability {
	return eventbus.QueueDriverCapability{
		SupportsBlockingDequeue: true,
		SupportsDelayQueue:      true,
		SupportsLease:           false,
		SupportsConsumerGroup:   true,
	}
}

func (d *driver) Enqueue(ctx context.Context, message eventbus.TaskMessage) error {
	if err := validateMessage(message); err != nil {
		return err
	}
	if d.fallback == nil {
		return fmt.Errorf("kafka driver is not wired with broker adapter yet")
	}
	return d.fallback.Enqueue(ctx, message)
}

func (d *driver) Dequeue(ctx context.Context, request eventbus.DequeueRequest) ([]eventbus.TaskMessage, error) {
	if strings.TrimSpace(request.TenantKey) == "" || strings.TrimSpace(request.SubscriberID) == "" {
		return nil, fmt.Errorf("tenant key and subscriber id are required")
	}
	if d.fallback == nil {
		return nil, fmt.Errorf("kafka driver is not wired with broker adapter yet")
	}
	if err := d.ensureSession(ctx, request); err != nil {
		return nil, err
	}
	return d.fallback.Dequeue(ctx, request)
}

func (d *driver) Ack(ctx context.Context, request eventbus.AckRequest) error {
	if d.fallback == nil {
		return fmt.Errorf("kafka driver is not wired with broker adapter yet")
	}
	if err := d.recordCommit(request, "ack"); err != nil {
		return err
	}
	return d.fallback.Ack(ctx, request)
}

func (d *driver) Nack(ctx context.Context, request eventbus.NackRequest) error {
	if d.fallback == nil {
		return fmt.Errorf("kafka driver is not wired with broker adapter yet")
	}
	if err := d.recordCommit(eventbus.AckRequest{TenantKey: request.TenantKey, SubscriberID: request.SubscriberID, MessageID: request.MessageID}, "nack"); err != nil {
		return err
	}
	return d.fallback.Nack(ctx, request)
}

func (d *driver) Retry(ctx context.Context, request eventbus.RetryRequest) error {
	if d.fallback == nil {
		return fmt.Errorf("kafka driver is not wired with broker adapter yet")
	}
	return d.fallback.Retry(ctx, request)
}

// CommitLog 返回内存提交记录快照（用于测试与调试）。
func (d *driver) CommitLog() []OffsetCommit {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]OffsetCommit, len(d.commits))
	copy(out, d.commits)
	return out
}

func (d *driver) ensureSession(ctx context.Context, request eventbus.DequeueRequest) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.runningSession != nil {
		return nil
	}
	session := &ConsumerGroupSession{
		GroupID:    d.consumerGroup,
		MemberID:   fmt.Sprintf("%s:%s", strings.TrimSpace(request.TenantKey), strings.TrimSpace(request.SubscriberID)),
		Generation: time.Now().UnixNano(),
		Assigned:   []int32{0},
	}
	d.runningSession = session
	d.rebalance.OnAssigned(ctx, *session)
	return nil
}

func (d *driver) recordCommit(request eventbus.AckRequest, metadata string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.runningSession == nil {
		return nil
	}
	d.commits = append(d.commits, OffsetCommit{
		Topic:     d.resolveTopic(request.TenantKey, request.SubscriberID),
		Partition: 0,
		Offset:    time.Now().UnixMilli(),
		Metadata:  metadata,
	})
	return nil
}

func (d *driver) resolveTopic(tenantKey, subscriberID string) string {
	tenant := strings.TrimSpace(tenantKey)
	if tenant == "" {
		tenant = "global"
	}
	subscriber := strings.TrimSpace(subscriberID)
	if subscriber == "" {
		subscriber = "default"
	}
	return fmt.Sprintf("%s.%s.%s", d.topicPrefix, tenant, subscriber)
}

func normalizeBrokers(brokers []string) []string {
	set := map[string]struct{}{}
	items := make([]string, 0, len(brokers))
	for _, broker := range brokers {
		value := strings.TrimSpace(broker)
		if value == "" {
			continue
		}
		if _, ok := set[value]; ok {
			continue
		}
		set[value] = struct{}{}
		items = append(items, value)
	}
	sort.Strings(items)
	return items
}

func validateMessage(message eventbus.TaskMessage) error {
	if strings.TrimSpace(message.ID) == "" {
		return fmt.Errorf("message id is required")
	}
	if strings.TrimSpace(message.TenantKey) == "" {
		return fmt.Errorf("tenant key is required")
	}
	if strings.TrimSpace(message.SubscriberID) == "" {
		return fmt.Errorf("subscriber id is required")
	}
	return nil
}
