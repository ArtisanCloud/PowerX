package event_bus

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type redisReliableQueue struct {
	client *redis.Client
}

// NewRedisReliableQueue 构建基于 Redis SortedSet 的可靠投递队列。
func NewRedisReliableQueue(client *redis.Client) ReliableQueue {
	return &redisReliableQueue{client: client}
}

func retryKey(tenantKey string) string {
	return fmt.Sprintf("event:retry:%s", tenantKey)
}

func leaseCounterKey(tenantKey, subscriberID string) string {
	return fmt.Sprintf("event:lease:%s:%s", tenantKey, subscriberID)
}

func leaseInstanceKey(tenantKey, subscriberID, leaseID string) string {
	return fmt.Sprintf("event:lease:%s:%s:%s", tenantKey, subscriberID, leaseID)
}

func (q *redisReliableQueue) ScheduleRetry(ctx context.Context, item RetryItem) error {
	if item.TenantKey == "" {
		return fmt.Errorf("tenant key is required")
	}
	if item.ExecuteAt.IsZero() {
		item.ExecuteAt = time.Now().UTC()
	}

	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}

	z := redis.Z{
		Score:  float64(item.ExecuteAt.UnixMilli()),
		Member: string(payload),
	}
	return q.client.ZAdd(ctx, retryKey(item.TenantKey), z).Err()
}

func (q *redisReliableQueue) PopDueRetries(ctx context.Context, tenantKey string, now time.Time, limit int) ([]RetryItem, error) {
	if tenantKey == "" {
		return nil, fmt.Errorf("tenant key is required")
	}
	if limit <= 0 {
		limit = 50
	}
	zRange := &redis.ZRangeBy{
		Min:    "-inf",
		Max:    fmt.Sprintf("%d", now.UTC().UnixMilli()),
		Offset: 0,
		Count:  int64(limit),
	}

	values, err := q.client.ZRangeByScoreWithScores(ctx, retryKey(tenantKey), zRange).Result()
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, nil
	}

	items := make([]RetryItem, 0, len(values))
	members := make([]interface{}, 0, len(values))

	for _, v := range values {
		var item RetryItem
		switch raw := v.Member.(type) {
		case string:
			if err := json.Unmarshal([]byte(raw), &item); err != nil {
				return nil, err
			}
			members = append(members, raw)
		case []byte:
			if err := json.Unmarshal(raw, &item); err != nil {
				return nil, err
			}
			members = append(members, string(raw))
		default:
			return nil, fmt.Errorf("unsupported retry member type %T", v.Member)
		}
		item.TenantKey = tenantKey
		item.ExecuteAt = time.UnixMilli(int64(v.Score)).UTC()
		items = append(items, item)
	}

	if err := q.client.ZRem(ctx, retryKey(tenantKey), members...).Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (q *redisReliableQueue) RemoveRetry(ctx context.Context, item RetryItem) error {
	if item.TenantKey == "" {
		return fmt.Errorf("tenant key is required")
	}

	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	return q.client.ZRem(ctx, retryKey(item.TenantKey), string(payload)).Err()
}

func (q *redisReliableQueue) AcquireLease(ctx context.Context, lease DeliveryLease) (bool, error) {
	if lease.TenantKey == "" || lease.SubscriberID == "" || lease.LeaseID == "" {
		return false, fmt.Errorf("tenant key, subscriber id and lease id are required")
	}

	if lease.MaxConcurrent <= 0 {
		lease.MaxConcurrent = 1
	}
	if lease.AckTimeout <= 0 {
		lease.AckTimeout = 30 * time.Second
	}

	instanceKey := leaseInstanceKey(lease.TenantKey, lease.SubscriberID, lease.LeaseID)
	counterKey := leaseCounterKey(lease.TenantKey, lease.SubscriberID)

	acquired, err := q.client.SetNX(ctx, instanceKey, "1", lease.AckTimeout).Result()
	if err != nil {
		return false, err
	}
	if !acquired {
		// 已持有租约，延长过期时间
		if err := q.client.Expire(ctx, instanceKey, lease.AckTimeout).Err(); err != nil {
			return false, err
		}
		return true, nil
	}

	count, err := q.client.Incr(ctx, counterKey).Result()
	if err != nil {
		_, _ = q.client.Del(ctx, instanceKey).Result()
		return false, err
	}
	if count == 1 {
		_ = q.client.Expire(ctx, counterKey, lease.AckTimeout).Err()
	}
	if count > int64(lease.MaxConcurrent) {
		_, _ = q.client.Decr(ctx, counterKey).Result()
		_, _ = q.client.Del(ctx, instanceKey).Result()
		return false, nil
	}
	return true, nil
}

func (q *redisReliableQueue) ReleaseLease(ctx context.Context, lease DeliveryLease) error {
	if lease.TenantKey == "" || lease.SubscriberID == "" || lease.LeaseID == "" {
		return fmt.Errorf("tenant key, subscriber id and lease id are required")
	}

	instanceKey := leaseInstanceKey(lease.TenantKey, lease.SubscriberID, lease.LeaseID)
	counterKey := leaseCounterKey(lease.TenantKey, lease.SubscriberID)

	removed, err := q.client.Del(ctx, instanceKey).Result()
	if err != nil {
		return err
	}
	if removed == 0 {
		return nil
	}

	val, err := q.client.Decr(ctx, counterKey).Result()
	if err != nil {
		return err
	}
	if val <= 0 {
		_, _ = q.client.Del(ctx, counterKey).Result()
	} else if lease.AckTimeout > 0 {
		_ = q.client.Expire(ctx, counterKey, lease.AckTimeout).Err()
	}
	return nil
}
