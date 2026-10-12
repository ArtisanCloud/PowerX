package event_bus

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrLeaseBusy  = errors.New("task lease is held by another worker")
	ErrLeaseStale = errors.New("task lease fencing token is stale")
)

// TaskLease is independent of message delivery: the same task may be
// delivered at least once, but only the current fencing token can be renewed
// or acknowledged by a worker.
type TaskLease struct {
	TenantKey    string
	SubscriberID string
	MessageID    string
	Owner        string
	Token        uint64
	ExpiresAt    time.Time
}

type RedisTaskLeases struct {
	client redis.UniversalClient
	now    func() time.Time
}

func NewRedisTaskLeases(client redis.UniversalClient) (*RedisTaskLeases, error) {
	if client == nil {
		return nil, fmt.Errorf("Redis task leases require a client")
	}
	return &RedisTaskLeases{client: client, now: time.Now}, nil
}

// Acquire allocates a monotonically increasing fencing token. Gaps are safe:
// losing SET NX races never grants the caller a usable lease.
func (m *RedisTaskLeases) Acquire(ctx context.Context, tenant, subscriber, messageID, owner string, ttl time.Duration) (TaskLease, error) {
	if m == nil || m.client == nil || !validLeaseParts(tenant, subscriber, messageID, owner, ttl) {
		return TaskLease{}, fmt.Errorf("invalid task lease request")
	}
	leaseKey, tokenKey := taskLeaseKeys(tenant, subscriber, messageID)
	token, err := m.client.Incr(ctx, tokenKey).Uint64()
	if err != nil {
		return TaskLease{}, err
	}
	value := leaseValue(owner, token)
	ok, err := m.client.SetNX(ctx, leaseKey, value, ttl).Result()
	if err != nil {
		return TaskLease{}, err
	}
	if !ok {
		return TaskLease{}, ErrLeaseBusy
	}
	return TaskLease{TenantKey: tenant, SubscriberID: subscriber, MessageID: messageID, Owner: owner, Token: token, ExpiresAt: m.now().Add(ttl)}, nil
}

var renewLeaseScript = redis.NewScript("if redis.call('GET', KEYS[1]) ~= ARGV[1] then return 0 end; return redis.call('PEXPIRE', KEYS[1], ARGV[2])")
var releaseLeaseScript = redis.NewScript("if redis.call('GET', KEYS[1]) ~= ARGV[1] then return 0 end; return redis.call('DEL', KEYS[1])")

func (m *RedisTaskLeases) Renew(ctx context.Context, lease TaskLease, ttl time.Duration) (TaskLease, error) {
	if m == nil || m.client == nil || !validLeaseParts(lease.TenantKey, lease.SubscriberID, lease.MessageID, lease.Owner, ttl) || lease.Token == 0 {
		return TaskLease{}, fmt.Errorf("invalid task lease")
	}
	key, _ := taskLeaseKeys(lease.TenantKey, lease.SubscriberID, lease.MessageID)
	updated, err := renewLeaseScript.Run(ctx, m.client, []string{key}, leaseValue(lease.Owner, lease.Token), ttl.Milliseconds()).Int64()
	if err != nil {
		return TaskLease{}, err
	}
	if updated != 1 {
		return TaskLease{}, ErrLeaseStale
	}
	lease.ExpiresAt = m.now().Add(ttl)
	return lease, nil
}

func (m *RedisTaskLeases) Release(ctx context.Context, lease TaskLease) error {
	if m == nil || m.client == nil || !validLeaseParts(lease.TenantKey, lease.SubscriberID, lease.MessageID, lease.Owner, time.Millisecond) || lease.Token == 0 {
		return fmt.Errorf("invalid task lease")
	}
	key, _ := taskLeaseKeys(lease.TenantKey, lease.SubscriberID, lease.MessageID)
	deleted, err := releaseLeaseScript.Run(ctx, m.client, []string{key}, leaseValue(lease.Owner, lease.Token)).Int64()
	if err != nil {
		return err
	}
	if deleted != 1 {
		return ErrLeaseStale
	}
	return nil
}

// Verify must be called before applying a worker result. RunStore must also
// perform its own CAS; this check alone cannot make business side effects safe.
func (m *RedisTaskLeases) Verify(ctx context.Context, lease TaskLease) error {
	if m == nil || m.client == nil || !validLeaseParts(lease.TenantKey, lease.SubscriberID, lease.MessageID, lease.Owner, time.Millisecond) || lease.Token == 0 {
		return fmt.Errorf("invalid task lease")
	}
	key, _ := taskLeaseKeys(lease.TenantKey, lease.SubscriberID, lease.MessageID)
	current, err := m.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) || current != leaseValue(lease.Owner, lease.Token) {
		return ErrLeaseStale
	}
	return err
}

func validLeaseParts(tenant, subscriber, messageID, owner string, ttl time.Duration) bool {
	return strings.TrimSpace(tenant) != "" && strings.TrimSpace(subscriber) != "" &&
		strings.TrimSpace(messageID) != "" && strings.TrimSpace(owner) != "" &&
		len(tenant) <= 256 && len(subscriber) <= 256 && len(messageID) <= 256 &&
		len(owner) <= 256 && ttl >= time.Millisecond
}

func taskLeaseKeys(tenant, subscriber, messageID string) (string, string) {
	scope := sha256.Sum256([]byte(tenant + "\x00" + subscriber))
	message := sha256.Sum256([]byte(messageID))
	tag := "{" + hex.EncodeToString(scope[:]) + "}"
	suffix := hex.EncodeToString(message[:])
	return "event_fabric:task:" + tag + ":lease:" + suffix, "event_fabric:task:" + tag + ":fence:" + suffix
}

func leaseValue(owner string, token uint64) string {
	return strconv.Itoa(len(owner)) + ":" + owner + ":" + strconv.FormatUint(token, 10)
}
