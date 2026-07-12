package event_bus

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultTaskQueuePrefix      = "event_fabric:task"
	defaultTaskDequeueTimeout   = 3 * time.Second
	defaultTaskProcessingExpiry = 30 * time.Minute
)

// RedisTaskDriverOptions 配置 Redis 任务驱动参数。
type RedisTaskDriverOptions struct {
	Client           *redis.Client
	Prefix           string
	BlockingTimeout  time.Duration
	ProcessingExpiry time.Duration
}

type redisTaskDriver struct {
	client           *redis.Client
	prefix           string
	blockingTimeout  time.Duration
	processingExpiry time.Duration
}

// NewRedisTaskDriver 构建默认的 Redis 任务驱动实现。
func NewRedisTaskDriver(opts RedisTaskDriverOptions) TaskDriver {
	prefix := strings.TrimSpace(opts.Prefix)
	if prefix == "" {
		prefix = defaultTaskQueuePrefix
	}
	blocking := opts.BlockingTimeout
	if blocking <= 0 {
		blocking = defaultTaskDequeueTimeout
	}
	expiry := opts.ProcessingExpiry
	if expiry <= 0 {
		expiry = defaultTaskProcessingExpiry
	}
	return &redisTaskDriver{
		client:           opts.Client,
		prefix:           prefix,
		blockingTimeout:  blocking,
		processingExpiry: expiry,
	}
}

func (d *redisTaskDriver) Type() QueueDriverType {
	return QueueDriverRedis
}

func (d *redisTaskDriver) Capability() QueueDriverCapability {
	return QueueDriverCapability{
		SupportsBlockingDequeue: true,
		SupportsDelayQueue:      true,
		SupportsLease:           false,
		SupportsConsumerGroup:   false,
	}
}

func (d *redisTaskDriver) Enqueue(ctx context.Context, message TaskMessage) error {
	if err := d.validateClient(); err != nil {
		return err
	}
	if err := validateTaskMessage(message); err != nil {
		return err
	}
	raw, err := json.Marshal(message)
	if err != nil {
		return err
	}

	queueKey := d.queueKey(message.TenantKey, message.SubscriberID)
	if !message.VisibleAt.IsZero() && message.VisibleAt.After(time.Now().UTC()) {
		return d.client.ZAdd(ctx, d.delayKey(message.TenantKey, message.SubscriberID), redis.Z{
			Score:  float64(message.VisibleAt.UnixMilli()),
			Member: string(raw),
		}).Err()
	}
	return d.client.LPush(ctx, queueKey, string(raw)).Err()
}

func (d *redisTaskDriver) Dequeue(ctx context.Context, request DequeueRequest) ([]TaskMessage, error) {
	if err := d.validateClient(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.TenantKey) == "" || strings.TrimSpace(request.SubscriberID) == "" {
		return nil, fmt.Errorf("tenant key and subscriber id are required")
	}
	maxItems := request.MaxItems
	if maxItems <= 0 {
		maxItems = 1
	}
	wait := request.WaitTimeout
	if wait <= 0 {
		wait = d.blockingTimeout
	}

	queueKey := d.queueKey(request.TenantKey, request.SubscriberID)
	processingKey := d.processingKey(request.TenantKey, request.SubscriberID)
	if err := d.flushDelayed(ctx, request.TenantKey, request.SubscriberID); err != nil {
		return nil, err
	}

	messages := make([]TaskMessage, 0, maxItems)
	firstRaw, err := d.client.BRPopLPush(ctx, queueKey, processingKey, wait).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}
	first, err := d.bindInflight(ctx, request.TenantKey, request.SubscriberID, firstRaw)
	if err != nil {
		return nil, err
	}
	messages = append(messages, first)

	for len(messages) < maxItems {
		raw, popErr := d.client.RPopLPush(ctx, queueKey, processingKey).Result()
		if popErr != nil {
			if popErr == redis.Nil {
				break
			}
			return nil, popErr
		}
		msg, bindErr := d.bindInflight(ctx, request.TenantKey, request.SubscriberID, raw)
		if bindErr != nil {
			return nil, bindErr
		}
		messages = append(messages, msg)
	}

	return messages, nil
}

func (d *redisTaskDriver) Ack(ctx context.Context, request AckRequest) error {
	if err := d.validateClient(); err != nil {
		return err
	}
	if strings.TrimSpace(request.TenantKey) == "" || strings.TrimSpace(request.SubscriberID) == "" || strings.TrimSpace(request.MessageID) == "" {
		return fmt.Errorf("tenant key, subscriber id and message id are required")
	}
	inflightKey := d.inflightKey(request.TenantKey, request.SubscriberID)
	processingKey := d.processingKey(request.TenantKey, request.SubscriberID)
	raw, err := d.client.HGet(ctx, inflightKey, request.MessageID).Result()
	if err != nil {
		if err == redis.Nil {
			if err := d.removeProcessingByMessageID(ctx, processingKey, request.MessageID); err != nil {
				return err
			}
			return d.client.HDel(ctx, inflightKey, request.MessageID).Err()
		}
		return err
	}
	if err := d.client.LRem(ctx, processingKey, 1, raw).Err(); err != nil {
		return err
	}
	return d.client.HDel(ctx, inflightKey, request.MessageID).Err()
}

func (d *redisTaskDriver) Nack(ctx context.Context, request NackRequest) error {
	if err := d.validateClient(); err != nil {
		return err
	}
	if strings.TrimSpace(request.TenantKey) == "" || strings.TrimSpace(request.SubscriberID) == "" || strings.TrimSpace(request.MessageID) == "" {
		return fmt.Errorf("tenant key, subscriber id and message id are required")
	}
	inflightKey := d.inflightKey(request.TenantKey, request.SubscriberID)
	processingKey := d.processingKey(request.TenantKey, request.SubscriberID)
	raw, err := d.client.HGet(ctx, inflightKey, request.MessageID).Result()
	if err != nil {
		if err == redis.Nil {
			if err := d.removeProcessingByMessageID(ctx, processingKey, request.MessageID); err != nil {
				return err
			}
			return d.client.HDel(ctx, inflightKey, request.MessageID).Err()
		}
		return err
	}
	if err := d.client.LRem(ctx, processingKey, 1, raw).Err(); err != nil {
		return err
	}
	if err := d.client.HDel(ctx, inflightKey, request.MessageID).Err(); err != nil {
		return err
	}

	if !request.RetryAt.IsZero() && request.RetryAt.After(time.Now().UTC()) {
		return d.client.ZAdd(ctx, d.delayKey(request.TenantKey, request.SubscriberID), redis.Z{
			Score:  float64(request.RetryAt.UnixMilli()),
			Member: raw,
		}).Err()
	}
	return d.client.LPush(ctx, d.queueKey(request.TenantKey, request.SubscriberID), raw).Err()
}

func (d *redisTaskDriver) removeProcessingByMessageID(ctx context.Context, processingKey, messageID string) error {
	items, err := d.client.LRange(ctx, processingKey, 0, -1).Result()
	if err != nil {
		return err
	}
	for _, raw := range items {
		var message TaskMessage
		if err := json.Unmarshal([]byte(raw), &message); err != nil {
			continue
		}
		if strings.TrimSpace(message.ID) != strings.TrimSpace(messageID) {
			continue
		}
		if err := d.client.LRem(ctx, processingKey, 1, raw).Err(); err != nil {
			return err
		}
	}
	return nil
}

func (d *redisTaskDriver) Retry(ctx context.Context, request RetryRequest) error {
	if err := d.validateClient(); err != nil {
		return err
	}
	message := request.Message
	if message.VisibleAt.IsZero() {
		message.VisibleAt = request.RetryAt
	}
	return d.Enqueue(ctx, message)
}

func (d *redisTaskDriver) bindInflight(ctx context.Context, tenantKey, subscriberID, raw string) (TaskMessage, error) {
	var message TaskMessage
	if err := json.Unmarshal([]byte(raw), &message); err != nil {
		return TaskMessage{}, err
	}
	if strings.TrimSpace(message.ID) == "" {
		return TaskMessage{}, fmt.Errorf("task message id is required")
	}
	inflightKey := d.inflightKey(tenantKey, subscriberID)
	if err := d.client.HSet(ctx, inflightKey, message.ID, raw).Err(); err != nil {
		return TaskMessage{}, err
	}
	if err := d.client.Expire(ctx, inflightKey, d.processingExpiry).Err(); err != nil {
		return TaskMessage{}, err
	}
	return message, nil
}

func (d *redisTaskDriver) flushDelayed(ctx context.Context, tenantKey, subscriberID string) error {
	delayKey := d.delayKey(tenantKey, subscriberID)
	now := fmt.Sprintf("%d", time.Now().UTC().UnixMilli())
	items, err := d.client.ZRangeByScore(ctx, delayKey, &redis.ZRangeBy{Min: "-inf", Max: now, Offset: 0, Count: 100}).Result()
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	queueKey := d.queueKey(tenantKey, subscriberID)
	pipe := d.client.TxPipeline()
	members := make([]interface{}, 0, len(items))
	for _, item := range items {
		pipe.LPush(ctx, queueKey, item)
		members = append(members, item)
	}
	pipe.ZRem(ctx, delayKey, members...)
	_, err = pipe.Exec(ctx)
	return err
}

func (d *redisTaskDriver) queueKey(tenantKey, subscriberID string) string {
	return fmt.Sprintf("%s:q:%s:%s", d.prefix, tenantKey, subscriberID)
}

func (d *redisTaskDriver) processingKey(tenantKey, subscriberID string) string {
	return fmt.Sprintf("%s:p:%s:%s", d.prefix, tenantKey, subscriberID)
}

func (d *redisTaskDriver) inflightKey(tenantKey, subscriberID string) string {
	return fmt.Sprintf("%s:i:%s:%s", d.prefix, tenantKey, subscriberID)
}

func (d *redisTaskDriver) delayKey(tenantKey, subscriberID string) string {
	return fmt.Sprintf("%s:d:%s:%s", d.prefix, tenantKey, subscriberID)
}

func (d *redisTaskDriver) validateClient() error {
	if d.client == nil {
		return fmt.Errorf("redis client is required")
	}
	return nil
}

func validateTaskMessage(message TaskMessage) error {
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
