package rabbitmq

import (
	"context"
	"fmt"
	"strings"
	"time"

	eventbus "github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

// DriverOptions 描述 RabbitMQ 驱动配置。
type DriverOptions struct {
	URL            string
	Exchange       string
	QueuePrefix    string
	ConsumerTag    string
	Prefetch       int
	PollTimeout    time.Duration
	FallbackDriver eventbus.TaskDriver
}

type driver struct {
	url         string
	exchange    string
	queuePrefix string
	consumerTag string
	prefetch    int
	pollTimeout time.Duration
	fallback    eventbus.TaskDriver
}

// NewDriver 创建 RabbitMQ TaskDriver 适配层。
func NewDriver(opts DriverOptions) eventbus.TaskDriver {
	exchange := strings.TrimSpace(opts.Exchange)
	if exchange == "" {
		exchange = "event_fabric.task"
	}
	queuePrefix := strings.TrimSpace(opts.QueuePrefix)
	if queuePrefix == "" {
		queuePrefix = "event_fabric.task"
	}
	consumerTag := strings.TrimSpace(opts.ConsumerTag)
	if consumerTag == "" {
		consumerTag = "powerx.event_fabric"
	}
	prefetch := opts.Prefetch
	if prefetch <= 0 {
		prefetch = 50
	}
	pollTimeout := opts.PollTimeout
	if pollTimeout <= 0 {
		pollTimeout = time.Second
	}
	return &driver{
		url:         strings.TrimSpace(opts.URL),
		exchange:    exchange,
		queuePrefix: queuePrefix,
		consumerTag: consumerTag,
		prefetch:    prefetch,
		pollTimeout: pollTimeout,
		fallback:    opts.FallbackDriver,
	}
}

func (d *driver) Type() eventbus.QueueDriverType { return eventbus.QueueDriverRabbitMQ }

func (d *driver) Capability() eventbus.QueueDriverCapability {
	return eventbus.QueueDriverCapability{SupportsBlockingDequeue: true, SupportsDelayQueue: true, SupportsConsumerGroup: true}
}

func (d *driver) Enqueue(ctx context.Context, message eventbus.TaskMessage) error {
	if d.fallback == nil {
		return fmt.Errorf("rabbitmq driver is not wired with broker adapter yet")
	}
	return d.fallback.Enqueue(ctx, message)
}

func (d *driver) Dequeue(ctx context.Context, request eventbus.DequeueRequest) ([]eventbus.TaskMessage, error) {
	if d.fallback == nil {
		return nil, fmt.Errorf("rabbitmq driver is not wired with broker adapter yet")
	}
	return d.fallback.Dequeue(ctx, request)
}

func (d *driver) Ack(ctx context.Context, request eventbus.AckRequest) error {
	if d.fallback == nil {
		return fmt.Errorf("rabbitmq driver is not wired with broker adapter yet")
	}
	return d.fallback.Ack(ctx, request)
}

func (d *driver) Nack(ctx context.Context, request eventbus.NackRequest) error {
	if d.fallback == nil {
		return fmt.Errorf("rabbitmq driver is not wired with broker adapter yet")
	}
	return d.fallback.Nack(ctx, request)
}

func (d *driver) Retry(ctx context.Context, request eventbus.RetryRequest) error {
	if d.fallback == nil {
		return fmt.Errorf("rabbitmq driver is not wired with broker adapter yet")
	}
	return d.fallback.Retry(ctx, request)
}
