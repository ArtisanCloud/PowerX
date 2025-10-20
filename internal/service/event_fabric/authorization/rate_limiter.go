package authorization

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

// RateLimiter 定义评估阶段的速率限制器。
type RateLimiter interface {
	Allow(ctx context.Context, key string, policy RateLimitPolicy) (RateLimitResult, error)
}

// RateLimitPolicy 描述限流策略。
type RateLimitPolicy struct {
	Limit    uint64
	Burst    uint64
	Interval time.Duration
}

// RateLimitResult 返回限流判定结果。
type RateLimitResult struct {
	Allowed    bool
	Remaining  int64
	ResetAfter time.Duration
}

// RateLimiterOptions 控制 Redis 限流实现。
type RateLimiterOptions struct {
	Client *redis.Client
	Prefix string
	Logger *pxlog.Logger
	Clock  func() time.Time
}

type redisRateLimiter struct {
	client *redis.Client
	prefix string
	logger *pxlog.Logger
	clock  func() time.Time
}

type noopRateLimiter struct{}

// NewRateLimiter 根据配置返回合适的限流器。
func NewRateLimiter(opts RateLimiterOptions) RateLimiter {
	if opts.Client == nil {
		return noopRateLimiter{}
	}
	prefix := strings.TrimSpace(opts.Prefix)
	if prefix == "" {
		prefix = "event_fabric:authorization:rl"
	}
	prefix = strings.TrimSuffix(prefix, ":")
	logger := opts.Logger
	if logger == nil {
		logger = pxlog.GetGlobalLogger()
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	return &redisRateLimiter{
		client: opts.Client,
		prefix: prefix,
		logger: logger,
		clock:  clock,
	}
}

// NewNoopRateLimiter 返回空实现。
func NewNoopRateLimiter() RateLimiter {
	return noopRateLimiter{}
}

func (noopRateLimiter) Allow(_ context.Context, _ string, _ RateLimitPolicy) (RateLimitResult, error) {
	return RateLimitResult{Allowed: true, Remaining: -1}, nil
}

func (r *redisRateLimiter) Allow(ctx context.Context, key string, policy RateLimitPolicy) (RateLimitResult, error) {
	if r == nil || r.client == nil || policy.Limit == 0 {
		return RateLimitResult{Allowed: true, Remaining: -1}, nil
	}

	interval := policy.Interval
	if interval <= 0 {
		interval = time.Minute
	}

	threshold := int64(policy.Limit)
	if policy.Burst > 0 {
		threshold += int64(policy.Burst)
	}
	if threshold <= 0 {
		return RateLimitResult{Allowed: true, Remaining: -1}, nil
	}

	redisKey := fmt.Sprintf("%s:%s", r.prefix, strings.TrimSpace(key))
	count, err := r.client.Incr(ctx, redisKey).Result()
	if err != nil {
		return RateLimitResult{}, err
	}

	if count == 1 {
		if err := r.client.Expire(ctx, redisKey, interval).Err(); err != nil {
			r.logger.WarnF(ctx, "[authorization.rate_limiter] expire key failed key=%s err=%v", redisKey, err)
		}
	}

	allowed := count <= threshold
	remaining := threshold - count
	if remaining < 0 {
		remaining = 0
	}

	return RateLimitResult{
		Allowed:    allowed,
		Remaining:  remaining,
		ResetAfter: interval,
	}, nil
}
