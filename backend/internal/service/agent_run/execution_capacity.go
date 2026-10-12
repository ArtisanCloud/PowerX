package agent_run

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var (
	ErrTenantCapacity        = errors.New("capacity.tenant")
	ErrRunCapacity           = errors.New("capacity.run")
	ErrSchedulingPolicyDrift = errors.New("scheduling.policy_drift")
	ErrExecutionLeaseStale   = errors.New("execution capacity lease stale")
)

// ExecutionCapacityPolicy 是跨 Core 的任务执行配额；模型物理槽仍独立治理。
type ExecutionCapacityPolicy struct {
	TenantLimit int
	RunLimit    int
	LeaseTTL    time.Duration
}

func (p ExecutionCapacityPolicy) Validate() error {
	if p.TenantLimit < 1 || p.TenantLimit > 10000 || p.RunLimit < 1 || p.RunLimit > p.TenantLimit || p.LeaseTTL < time.Second || p.LeaseTTL > time.Minute {
		return ErrInvalid
	}
	return nil
}
func (p ExecutionCapacityPolicy) fingerprint() string {
	return fmt.Sprintf("%d:%d:%d", p.TenantLimit, p.RunLimit, p.LeaseTTL.Milliseconds())
}

type ExecutionCapacity struct {
	client redis.UniversalClient
	policy ExecutionCapacityPolicy
	env    string
}
type ExecutionLease struct {
	Identity Snapshot
	Token    string
}

// NewExecutionCapacity 发布环境级固定策略。部署配置漂移不能静默扩大配额。
func NewExecutionCapacity(ctx context.Context, client redis.UniversalClient, env string, policy ExecutionCapacityPolicy) (*ExecutionCapacity, error) {
	if client == nil || !validArchiveEnv(env) || policy.Validate() != nil {
		return nil, ErrInvalid
	}
	key := "agent:scheduling:{" + env + "}:policy"
	if err := client.SetNX(ctx, key, policy.fingerprint(), 0).Err(); err != nil {
		return nil, err
	}
	value, err := client.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if value != policy.fingerprint() {
		return nil, ErrSchedulingPolicyDrift
	}
	return &ExecutionCapacity{client: client, policy: policy, env: env}, nil
}

func executionCapacityKeys(id Snapshot) []string {
	// 两级额度必须处于同一租户分片，禁止先占 tenant 再等待 run 的占槽死锁。
	base := "agent:execution:{" + id.TenantUUID + ":" + id.Env + "}"
	return []string{base + ":tenant", base + ":run:" + id.RunID, base + ":policy"}
}

var acquireExecutionCapacity = redis.NewScript(`
local p = redis.call('GET', KEYS[3])
if p and p ~= ARGV[1] then return -3 end
local clock = redis.call('TIME'); local now = tonumber(clock[1])*1000 + math.floor(tonumber(clock[2])/1000)
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now)
redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', now)
if redis.call('ZCARD', KEYS[2]) >= tonumber(ARGV[3]) then return -2 end
if redis.call('ZCARD', KEYS[1]) >= tonumber(ARGV[2]) then return -1 end
local expires = now + tonumber(ARGV[4])
redis.call('ZADD', KEYS[1], expires, ARGV[5]); redis.call('ZADD', KEYS[2], expires, ARGV[5])
redis.call('SET', KEYS[3], ARGV[1], 'PX', tonumber(ARGV[4])*2)
redis.call('PEXPIRE', KEYS[1], tonumber(ARGV[4])*2); redis.call('PEXPIRE', KEYS[2], tonumber(ARGV[4])*2)
return expires`)

func (c *ExecutionCapacity) Acquire(ctx context.Context, id Snapshot) (ExecutionLease, error) {
	if c == nil || !validScope(id) || id.Env != c.env {
		return ExecutionLease{}, ErrInvalid
	}
	lease := ExecutionLease{Identity: id, Token: uuid.NewString()}
	n, err := acquireExecutionCapacity.Run(ctx, c.client, executionCapacityKeys(id), c.policy.fingerprint(), c.policy.TenantLimit, c.policy.RunLimit, c.policy.LeaseTTL.Milliseconds(), lease.Token).Int64()
	if err != nil {
		return ExecutionLease{}, err
	}
	switch n {
	case -1:
		return ExecutionLease{}, ErrTenantCapacity
	case -2:
		return ExecutionLease{}, ErrRunCapacity
	case -3:
		return ExecutionLease{}, ErrSchedulingPolicyDrift
	}
	return lease, nil
}

var renewExecutionCapacity = redis.NewScript(`
local clock = redis.call('TIME'); local now = tonumber(clock[1])*1000 + math.floor(tonumber(clock[2])/1000)
local t = redis.call('ZSCORE', KEYS[1], ARGV[1]); local r = redis.call('ZSCORE', KEYS[2], ARGV[1])
if not t or not r or tonumber(t) <= now or tonumber(r) <= now then return 0 end
if redis.call('GET', KEYS[3]) ~= ARGV[3] then return 0 end
local expires = now + tonumber(ARGV[2])
redis.call('ZADD', KEYS[1], expires, ARGV[1]); redis.call('ZADD', KEYS[2], expires, ARGV[1])
for _,key in ipairs(KEYS) do redis.call('PEXPIRE',key,tonumber(ARGV[2])*2) end
return 1`)

func (c *ExecutionCapacity) Renew(ctx context.Context, lease ExecutionLease) error {
	if c == nil || !validScope(lease.Identity) || lease.Identity.Env != c.env || lease.Token == "" {
		return ErrInvalid
	}
	n, err := renewExecutionCapacity.Run(ctx, c.client, executionCapacityKeys(lease.Identity), lease.Token, c.policy.LeaseTTL.Milliseconds(), c.policy.fingerprint()).Int64()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrExecutionLeaseStale
	}
	return nil
}

var releaseExecutionCapacity = redis.NewScript(`redis.call('ZREM', KEYS[1], ARGV[1]); return redis.call('ZREM', KEYS[2], ARGV[1])`)

func (c *ExecutionCapacity) Release(ctx context.Context, lease ExecutionLease) error {
	if c == nil || !validScope(lease.Identity) || lease.Identity.Env != c.env || lease.Token == "" {
		return ErrInvalid
	}
	return releaseExecutionCapacity.Run(ctx, c.client, executionCapacityKeys(lease.Identity)[:2], lease.Token).Err()
}

// Execute 在续租失败时取消执行上下文，旧 Worker 不得继续发起业务调用。
func (c *ExecutionCapacity) Execute(ctx context.Context, lease ExecutionLease, execute func(context.Context) error) error {
	if c == nil || !validScope(lease.Identity) || lease.Identity.Env != c.env || lease.Token == "" || execute == nil {
		return ErrInvalid
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	renewed := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(c.policy.LeaseTTL / 3)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				renewed <- nil
				return
			case <-runCtx.Done():
				renewed <- nil
				return
			case <-ticker.C:
				if err := c.Renew(runCtx, lease); err != nil {
					cancel()
					renewed <- err
					return
				}
			}
		}
	}()
	err := execute(runCtx)
	close(done)
	renewErr := <-renewed
	cleanup, cancelCleanup := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelCleanup()
	releaseErr := c.Release(cleanup, lease)
	return errors.Join(err, renewErr, releaseErr)
}
