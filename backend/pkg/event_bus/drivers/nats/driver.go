package nats

import (
	"context"
	"fmt"
	"strings"
	"time"

	eventbus "github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

// DriverOptions 描述 NATS 驱动配置。
type DriverOptions struct {
	URLs           []string
	SubjectPrefix  string
	QueueGroup     string
	PollTimeout    time.Duration
	FallbackDriver eventbus.TaskDriver
}

type driver struct {
	urls          []string
	subjectPrefix string
	queueGroup    string
	pollTimeout   time.Duration
	fallback      eventbus.TaskDriver
}

// NewDriver 创建 NATS TaskDriver 适配层。
func NewDriver(opts DriverOptions) eventbus.TaskDriver {
	subjectPrefix := strings.TrimSpace(opts.SubjectPrefix)
	if subjectPrefix == "" {
		subjectPrefix = "event_fabric.task"
	}
	queueGroup := strings.TrimSpace(opts.QueueGroup)
	if queueGroup == "" {
		queueGroup = "powerx.event_fabric"
	}
	pollTimeout := opts.PollTimeout
	if pollTimeout <= 0 {
		pollTimeout = time.Second
	}
	urls := make([]string, 0, len(opts.URLs))
	for _, item := range opts.URLs {
		value := strings.TrimSpace(item)
		if value != "" {
			urls = append(urls, value)
		}
	}
	return &driver{urls: urls, subjectPrefix: subjectPrefix, queueGroup: queueGroup, pollTimeout: pollTimeout, fallback: opts.FallbackDriver}
}

func (d *driver) Type() eventbus.QueueDriverType { return eventbus.QueueDriverNATS }

func (d *driver) Capability() eventbus.QueueDriverCapability {
	return eventbus.QueueDriverCapability{SupportsBlockingDequeue: true, SupportsDelayQueue: true, SupportsConsumerGroup: true}
}

func (d *driver) Enqueue(ctx context.Context, message eventbus.TaskMessage) error {
	if d.fallback == nil {
		return fmt.Errorf("nats driver is not wired with broker adapter yet")
	}
	return d.fallback.Enqueue(ctx, message)
}

func (d *driver) Dequeue(ctx context.Context, request eventbus.DequeueRequest) ([]eventbus.TaskMessage, error) {
	if d.fallback == nil {
		return nil, fmt.Errorf("nats driver is not wired with broker adapter yet")
	}
	return d.fallback.Dequeue(ctx, request)
}

func (d *driver) Ack(ctx context.Context, request eventbus.AckRequest) error {
	if d.fallback == nil {
		return fmt.Errorf("nats driver is not wired with broker adapter yet")
	}
	return d.fallback.Ack(ctx, request)
}

func (d *driver) Nack(ctx context.Context, request eventbus.NackRequest) error {
	if d.fallback == nil {
		return fmt.Errorf("nats driver is not wired with broker adapter yet")
	}
	return d.fallback.Nack(ctx, request)
}

func (d *driver) Retry(ctx context.Context, request eventbus.RetryRequest) error {
	if d.fallback == nil {
		return fmt.Errorf("nats driver is not wired with broker adapter yet")
	}
	return d.fallback.Retry(ctx, request)
}
