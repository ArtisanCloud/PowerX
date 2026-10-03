package event_bus

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// StreamTaskDelivery is a leased, at-least-once delivery. The worker must
// carry Lease.Token into its RunStore result CAS before acknowledging it.
type StreamTaskDelivery struct {
	Message  TaskMessage
	StreamID string
	Group    string
	Lease    TaskLease
}

type RedisStreamTaskDriver struct {
	client    redis.UniversalClient
	leases    *RedisTaskLeases
	leaseTTL  time.Duration
	dedupeTTL time.Duration
}

type StreamNackOutcome string

const (
	StreamRetried StreamNackOutcome = "retried"
	StreamDead    StreamNackOutcome = "dead"
)

func NewRedisStreamTaskDriver(client redis.UniversalClient, leaseTTL time.Duration) (*RedisStreamTaskDriver, error) {
	if client == nil || leaseTTL < time.Second {
		return nil, fmt.Errorf("Redis Stream task driver needs a client and lease TTL >= 1s")
	}
	leases, err := NewRedisTaskLeases(client)
	if err != nil {
		return nil, err
	}
	return &RedisStreamTaskDriver{client: client, leases: leases, leaseTTL: leaseTTL, dedupeTTL: 24 * time.Hour}, nil
}

func (d *RedisStreamTaskDriver) Type() QueueDriverType { return QueueDriverRedis }

func (d *RedisStreamTaskDriver) Capability() QueueDriverCapability {
	return QueueDriverCapability{
		SupportsBlockingDequeue: true, SupportsDelayQueue: true,
		SupportsLease: true, SupportsConsumerGroup: true,
	}
}

// Enqueue is idempotent for a message ID within the configured dedupe window.
// The dedupe key and Stream share one Redis Cluster hash slot.
func (d *RedisStreamTaskDriver) Enqueue(ctx context.Context, message TaskMessage) error {
	if d == nil || d.client == nil || validateTaskMessage(message) != nil {
		return fmt.Errorf("invalid Stream task message")
	}
	raw, err := json.Marshal(message)
	if err != nil {
		return err
	}
	stream := taskStreamKey(message.TenantKey, message.SubscriberID)
	dedupe := taskDedupeKey(message.TenantKey, message.SubscriberID, message.ID)
	for attempt := 0; attempt < 5; attempt++ {
		err := d.client.Watch(ctx, func(tx *redis.Tx) error {
			exists, err := tx.Exists(ctx, dedupe).Result()
			if err != nil || exists > 0 {
				return err
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, dedupe, "1", d.dedupeTTL)
				if message.VisibleAt.After(time.Now()) {
					base := strings.TrimSuffix(stream, ":stream")
					pipe.ZAdd(ctx, base+":delay", redis.Z{Score: float64(message.VisibleAt.UnixMilli()), Member: string(raw)})
				} else {
					pipe.XAdd(ctx, &redis.XAddArgs{Stream: stream, Values: map[string]any{"message": string(raw)}})
				}
				return nil
			})
			return err
		}, dedupe)
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		return err
	}
	return ErrLeaseBusy
}

// Dequeue first reclaims expired pending entries, then blocks for new work.
// A crash between group delivery and lease acquisition is recovered by the
// next XAUTOCLAIM after leaseTTL.
func (d *RedisStreamTaskDriver) Dequeue(ctx context.Context, tenant, subscriber, group, worker string, max int64, wait time.Duration) ([]StreamTaskDelivery, error) {
	if d == nil || d.client == nil || !validLeaseParts(tenant, subscriber, "queue", worker, d.leaseTTL) ||
		strings.TrimSpace(group) == "" || max < 1 || max > 100 || wait < 0 {
		return nil, fmt.Errorf("invalid Stream dequeue request")
	}
	if wait == 0 {
		wait = 3 * time.Second
	}
	if wait > 30*time.Second {
		wait = 30 * time.Second
	}
	stream := taskStreamKey(tenant, subscriber)
	if err := d.client.XGroupCreateMkStream(ctx, stream, group, "0").Err(); err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return nil, err
	}
	messages, _, err := d.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream: stream, Group: group, Consumer: worker,
		MinIdle: d.leaseTTL, Start: "0-0", Count: max,
	}).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	if len(messages) == 0 {
		streams, readErr := d.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group: group, Consumer: worker, Streams: []string{stream, ">"},
			Count: max, Block: wait,
		}).Result()
		if errors.Is(readErr, redis.Nil) {
			return nil, nil
		}
		if readErr != nil {
			return nil, readErr
		}
		for _, item := range streams {
			messages = append(messages, item.Messages...)
		}
	}
	deliveries := make([]StreamTaskDelivery, 0, len(messages))
	for _, entry := range messages {
		raw, ok := entry.Values["message"].(string)
		if !ok {
			return deliveries, fmt.Errorf("Stream task %s lacks message", entry.ID)
		}
		var message TaskMessage
		if err := json.Unmarshal([]byte(raw), &message); err != nil {
			return deliveries, err
		}
		if message.TenantKey != tenant || message.SubscriberID != subscriber {
			return deliveries, fmt.Errorf("Stream task scope mismatch")
		}
		lease, err := d.leases.Acquire(ctx, tenant, subscriber, message.ID, worker, d.leaseTTL)
		if errors.Is(err, ErrLeaseBusy) {
			continue
		}
		if err != nil {
			return deliveries, err
		}
		deliveries = append(deliveries, StreamTaskDelivery{Message: message, StreamID: entry.ID, Group: group, Lease: lease})
	}
	return deliveries, nil
}

var nackStreamTaskScript = redis.NewScript(
	"if redis.call('GET', KEYS[4]) ~= ARGV[3] then return 0 end; " +
		"local n = redis.call('XACK', KEYS[1], ARGV[1], ARGV[2]); " +
		"if n ~= 1 then return 0 end; " +
		"redis.call('XDEL', KEYS[1], ARGV[2]); redis.call('DEL', KEYS[4]); " +
		"if ARGV[4] == 'dead' then redis.call('XADD', KEYS[3], '*', 'message', ARGV[5], 'reason', ARGV[6]); " +
		"elseif ARGV[4] == 'delayed' then redis.call('ZADD', KEYS[2], ARGV[7], ARGV[5]); " +
		"else redis.call('XADD', KEYS[1], '*', 'message', ARGV[5]); end; return 1")

// Nack moves a failed delivery to delayed retry, immediate retry or DLQ in
// one same-slot transaction. maxAttempts includes the first delivery.
func (d *RedisStreamTaskDriver) Nack(ctx context.Context, delivery StreamTaskDelivery, retryAt time.Time, reason string, maxAttempts int) (StreamNackOutcome, error) {
	if d == nil || d.client == nil || delivery.StreamID == "" || delivery.Group == "" ||
		delivery.Lease.Token == 0 || maxAttempts < 1 || maxAttempts > 100 ||
		strings.TrimSpace(reason) == "" || len(reason) > 128 {
		return "", fmt.Errorf("invalid Stream task NACK")
	}
	message := delivery.Message
	if message.ID != delivery.Lease.MessageID || message.TenantKey != delivery.Lease.TenantKey ||
		message.SubscriberID != delivery.Lease.SubscriberID {
		return "", fmt.Errorf("Stream task lease does not match message")
	}
	message.Attempt++
	message.VisibleAt = retryAt.UTC()
	raw, err := json.Marshal(message)
	if err != nil {
		return "", err
	}
	mode, outcome := "immediate", StreamRetried
	if message.Attempt >= maxAttempts {
		mode, outcome = "dead", StreamDead
	} else if retryAt.After(time.Now()) {
		mode = "delayed"
	}
	stream := taskStreamKey(message.TenantKey, message.SubscriberID)
	base := strings.TrimSuffix(stream, ":stream")
	leaseKey, _ := taskLeaseKeys(message.TenantKey, message.SubscriberID, message.ID)
	n, err := nackStreamTaskScript.Run(ctx, d.client,
		[]string{stream, base + ":delay", base + ":dead", leaseKey},
		delivery.Group, delivery.StreamID, leaseValue(delivery.Lease.Owner, delivery.Lease.Token),
		mode, string(raw), reason, retryAt.UnixMilli()).Int64()
	if err != nil {
		return "", err
	}
	if n != 1 {
		return "", ErrLeaseStale
	}
	return outcome, nil
}

var promoteDelayedScript = redis.NewScript(
	"local items = redis.call('ZRANGEBYSCORE', KEYS[2], '-inf', ARGV[1], 'LIMIT', 0, ARGV[2]); " +
		"for _, raw in ipairs(items) do redis.call('XADD', KEYS[1], '*', 'message', raw); redis.call('ZREM', KEYS[2], raw); end; return #items")

func (d *RedisStreamTaskDriver) PromoteDue(ctx context.Context, tenant, subscriber string, limit int) (int64, error) {
	if d == nil || d.client == nil || strings.TrimSpace(tenant) == "" || strings.TrimSpace(subscriber) == "" || limit < 1 || limit > 1000 {
		return 0, fmt.Errorf("invalid delayed task promotion")
	}
	stream := taskStreamKey(tenant, subscriber)
	base := strings.TrimSuffix(stream, ":stream")
	return promoteDelayedScript.Run(ctx, d.client, []string{stream, base + ":delay"}, time.Now().UnixMilli(), limit).Int64()
}

var renewStreamTaskScript = redis.NewScript(
	"if redis.call('GET', KEYS[2]) ~= ARGV[4] then return 0 end; " +
		"local ids = redis.call('XCLAIM', KEYS[1], ARGV[1], ARGV[2], 0, ARGV[3], 'JUSTID'); " +
		"if #ids ~= 1 then return 0 end; return redis.call('PEXPIRE', KEYS[2], ARGV[5])")

func (d *RedisStreamTaskDriver) Renew(ctx context.Context, delivery StreamTaskDelivery) (StreamTaskDelivery, error) {
	if d == nil || d.client == nil || delivery.StreamID == "" || delivery.Group == "" || delivery.Lease.Token == 0 {
		return StreamTaskDelivery{}, fmt.Errorf("Stream task driver is unavailable")
	}
	stream := taskStreamKey(delivery.Lease.TenantKey, delivery.Lease.SubscriberID)
	leaseKey, _ := taskLeaseKeys(delivery.Lease.TenantKey, delivery.Lease.SubscriberID, delivery.Lease.MessageID)
	n, err := renewStreamTaskScript.Run(ctx, d.client, []string{stream, leaseKey},
		delivery.Group, delivery.Lease.Owner, delivery.StreamID,
		leaseValue(delivery.Lease.Owner, delivery.Lease.Token), d.leaseTTL.Milliseconds()).Int64()
	if err != nil {
		return StreamTaskDelivery{}, err
	}
	if n != 1 {
		return StreamTaskDelivery{}, ErrLeaseStale
	}
	delivery.Lease.ExpiresAt = time.Now().Add(d.leaseTTL)
	return delivery, nil
}

var ackStreamTaskScript = redis.NewScript(
	"if redis.call('GET', KEYS[2]) ~= ARGV[3] then return 0 end; " +
		"local n = redis.call('XACK', KEYS[1], ARGV[1], ARGV[2]); " +
		"if n == 1 then redis.call('XDEL', KEYS[1], ARGV[2]); redis.call('DEL', KEYS[2]); end; return n")

// Ack refuses stale fencing tokens and deletes the Stream entry only after
// the current worker's result has been persisted by its caller.
func (d *RedisStreamTaskDriver) Ack(ctx context.Context, delivery StreamTaskDelivery) error {
	if d == nil || d.client == nil || delivery.StreamID == "" || delivery.Group == "" || delivery.Lease.Token == 0 {
		return fmt.Errorf("invalid Stream task acknowledgement")
	}
	stream := taskStreamKey(delivery.Lease.TenantKey, delivery.Lease.SubscriberID)
	leaseKey, _ := taskLeaseKeys(delivery.Lease.TenantKey, delivery.Lease.SubscriberID, delivery.Lease.MessageID)
	n, err := ackStreamTaskScript.Run(ctx, d.client, []string{stream, leaseKey},
		delivery.Group, delivery.StreamID, leaseValue(delivery.Lease.Owner, delivery.Lease.Token)).Int64()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrLeaseStale
	}
	return nil
}

func taskStreamKey(tenant, subscriber string) string {
	sum := sha256.Sum256([]byte(tenant + "\x00" + subscriber))
	return "event_fabric:task:{" + hex.EncodeToString(sum[:]) + "}:stream"
}

func taskDedupeKey(tenant, subscriber, messageID string) string {
	sum := sha256.Sum256([]byte(messageID))
	stream := taskStreamKey(tenant, subscriber)
	return strings.TrimSuffix(stream, ":stream") + ":dedupe:" + hex.EncodeToString(sum[:])
}
