package agent_run

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var (
	ErrModelPoolFull     = errors.New("model pool is at physical capacity")
	ErrPoolCapacityDrift = errors.New("model pool capacity differs from the published deployment")
	ErrModelLeaseStale   = errors.New("model pool lease expired or was released")
	ErrModelQueueFull    = errors.New("model pool waiting queue is full")
)

type ModelLease struct {
	PoolID    string
	Owner     string
	Token     string
	ExpiresAt time.Time
}

type RedisModelPool struct {
	client redis.UniversalClient
}

func NewRedisModelPool(client redis.UniversalClient) (*RedisModelPool, error) {
	if client == nil {
		return nil, fmt.Errorf("model pool requires Redis")
	}
	return &RedisModelPool{client: client}, nil
}

var acquireModelSlotScript = redis.NewScript(
	"local published = redis.call('GET', KEYS[2]); " +
		"if published and tonumber(published) ~= tonumber(ARGV[1]) then return -1 end; " +
		"if not published then redis.call('SET', KEYS[2], ARGV[1]) end; " +
		"local clock = redis.call('TIME'); local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000); " +
		"redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now); " +
		"local expired = redis.call('ZRANGEBYSCORE', KEYS[4], '-inf', now); " +
		"for _, token in ipairs(expired) do redis.call('ZREM', KEYS[3], token); redis.call('ZREM', KEYS[4], token) end; " +
		"if redis.call('ZCARD', KEYS[3]) > 0 then return 0 end; " +
		"if redis.call('ZCARD', KEYS[1]) >= tonumber(ARGV[1]) then return 0 end; " +
		"local expires = now + tonumber(ARGV[2]); redis.call('ZADD', KEYS[1], expires, ARGV[3]); return expires")

// TryAcquire uses an explicit physical pool capacity shared across all Core
// instances and tenants. It never derives capacity from a tenant Model Profile.
func (p *RedisModelPool) TryAcquire(ctx context.Context, poolID, owner string, capacity int, ttl time.Duration) (ModelLease, error) {
	if p == nil || p.client == nil || !validModelPoolArgs(poolID, owner, capacity, ttl) {
		return ModelLease{}, ErrInvalid
	}
	token := owner + ":" + uuid.NewString()
	slots, declared := modelPoolKeys(poolID)
	order, expiry, _ := modelWaitKeys(poolID)
	expires, err := acquireModelSlotScript.Run(ctx, p.client, []string{slots, declared, order, expiry},
		capacity, ttl.Milliseconds(), token).Int64()
	if err != nil {
		return ModelLease{}, err
	}
	switch expires {
	case -1:
		return ModelLease{}, ErrPoolCapacityDrift
	case 0:
		return ModelLease{}, ErrModelPoolFull
	}
	return ModelLease{PoolID: poolID, Owner: owner, Token: token, ExpiresAt: time.UnixMilli(expires).UTC()}, nil
}

var waitModelSlotScript = redis.NewScript(
	"local published = redis.call('GET', KEYS[2]); " +
		"if published and tonumber(published) ~= tonumber(ARGV[1]) then return -1 end; " +
		"if not published then redis.call('SET', KEYS[2], ARGV[1]) end; " +
		"local clock = redis.call('TIME'); local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000); " +
		"redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now); " +
		"local expired = redis.call('ZRANGEBYSCORE', KEYS[4], '-inf', now); " +
		"for _, token in ipairs(expired) do redis.call('ZREM', KEYS[3], token); redis.call('ZREM', KEYS[4], token) end; " +
		"if not redis.call('ZSCORE', KEYS[3], ARGV[3]) then " +
		"if redis.call('ZCARD', KEYS[3]) >= tonumber(ARGV[4]) then return -2 end; " +
		"local seq = redis.call('INCR', KEYS[5]); redis.call('ZADD', KEYS[3], seq, ARGV[3]); end; " +
		"redis.call('ZADD', KEYS[4], now + tonumber(ARGV[5]), ARGV[3]); " +
		"local head = redis.call('ZRANGE', KEYS[3], 0, 0)[1]; " +
		"if head ~= ARGV[3] or redis.call('ZCARD', KEYS[1]) >= tonumber(ARGV[1]) then return 0 end; " +
		"redis.call('ZREM', KEYS[3], ARGV[3]); redis.call('ZREM', KEYS[4], ARGV[3]); " +
		"local expires = now + tonumber(ARGV[2]); redis.call('ZADD', KEYS[1], expires, ARGV[3]); return expires")

var cancelModelWaitScript = redis.NewScript(
	"redis.call('ZREM', KEYS[1], ARGV[1]); return redis.call('ZREM', KEYS[2], ARGV[1])")

// WaitAcquire grants slots in FIFO order across Core instances and tenants.
// The caller's context is the queue-wait budget; the provider request budget
// must start only after this method returns a lease. Waiting entries expire
// if a process crashes before it can remove its token.
func (p *RedisModelPool) WaitAcquire(ctx context.Context, poolID, owner string, capacity, maxWaiting int, ttl time.Duration) (ModelLease, error) {
	if p == nil || p.client == nil || !validModelPoolArgs(poolID, owner, capacity, ttl) || maxWaiting < 1 || maxWaiting > 100000 {
		return ModelLease{}, ErrInvalid
	}
	token := owner + ":" + uuid.NewString()
	slots, declared := modelPoolKeys(poolID)
	order, expiry, seq := modelWaitKeys(poolID)
	keys := []string{slots, declared, order, expiry, seq}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = cancelModelWaitScript.Run(cleanupCtx, p.client, []string{order, expiry}, token).Err()
	}()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return ModelLease{}, err
		}
		expires, err := waitModelSlotScript.Run(ctx, p.client, keys,
			capacity, ttl.Milliseconds(), token, maxWaiting, 5000).Int64()
		if err != nil {
			return ModelLease{}, err
		}
		switch expires {
		case -1:
			return ModelLease{}, ErrPoolCapacityDrift
		case -2:
			return ModelLease{}, ErrModelQueueFull
		case 0:
			select {
			case <-ctx.Done():
				return ModelLease{}, ctx.Err()
			case <-ticker.C:
			}
		default:
			return ModelLease{PoolID: poolID, Owner: owner, Token: token, ExpiresAt: time.UnixMilli(expires).UTC()}, nil
		}
	}
}

var renewModelSlotScript = redis.NewScript(
	"local clock = redis.call('TIME'); local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000); " +
		"local expires = redis.call('ZSCORE', KEYS[1], ARGV[1]); " +
		"if not expires or tonumber(expires) <= now then return 0 end; " +
		"local next = now + tonumber(ARGV[2]); redis.call('ZADD', KEYS[1], next, ARGV[1]); return next")

func (p *RedisModelPool) Renew(ctx context.Context, lease ModelLease, ttl time.Duration) (ModelLease, error) {
	if p == nil || p.client == nil || !validModelPoolArgs(lease.PoolID, lease.Owner, 1, ttl) || lease.Token == "" {
		return ModelLease{}, ErrInvalid
	}
	slots, _ := modelPoolKeys(lease.PoolID)
	expires, err := renewModelSlotScript.Run(ctx, p.client, []string{slots}, lease.Token, ttl.Milliseconds()).Int64()
	if err != nil {
		return ModelLease{}, err
	}
	if expires == 0 {
		return ModelLease{}, ErrModelLeaseStale
	}
	lease.ExpiresAt = time.UnixMilli(expires).UTC()
	return lease, nil
}

func (p *RedisModelPool) Release(ctx context.Context, lease ModelLease) error {
	if p == nil || p.client == nil || strings.TrimSpace(lease.PoolID) == "" || lease.Token == "" {
		return ErrInvalid
	}
	slots, _ := modelPoolKeys(lease.PoolID)
	removed, err := p.client.ZRem(ctx, slots, lease.Token).Result()
	if err != nil {
		return err
	}
	if removed == 0 {
		return ErrModelLeaseStale
	}
	return nil
}

func validModelPoolArgs(poolID, owner string, capacity int, ttl time.Duration) bool {
	return strings.TrimSpace(poolID) != "" && len(poolID) <= 256 &&
		strings.TrimSpace(owner) != "" && len(owner) <= 256 &&
		capacity >= 1 && capacity <= 10000 && ttl >= time.Second && ttl <= time.Hour
}

func modelPoolKeys(poolID string) (string, string) {
	sum := sha256.Sum256([]byte(poolID))
	base := "agent:model:{" + hex.EncodeToString(sum[:]) + "}"
	return base + ":slots", base + ":capacity"
}

func modelWaitKeys(poolID string) (string, string, string) {
	slots, _ := modelPoolKeys(poolID)
	base := strings.TrimSuffix(slots, ":slots")
	return base + ":wait_order", base + ":wait_expiry", base + ":wait_sequence"
}
