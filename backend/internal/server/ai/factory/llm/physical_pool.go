package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	agentconfig "github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	modelconfig "github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/redis/go-redis/v9"
)

var (
	ErrPhysicalModelPoolUnconfigured = errors.New("physical model pool is not configured for Ollama target")
	ErrModelQueueTimeout             = errors.New("model physical pool queue wait timed out")
	ErrPhysicalModelPoolUnavailable  = errors.New("physical model pool is unavailable")
)

type physicalPoolRuntime struct {
	pool      *agent_run.RedisModelPool
	rules     map[string]agentconfig.PhysicalModelPool
	queueWait time.Duration
}

var configuredPhysicalPool atomic.Pointer[physicalPoolRuntime]

// ConfigurePhysicalModelPools binds deployment-owned Redis capacity to model
// calls. Configured local Ollama models fail closed if the exact endpoint/model
// has no rule. Production Redis durability is checked before publication.
func ConfigurePhysicalModelPools(ctx context.Context, client redis.UniversalClient, rules []agentconfig.PhysicalModelPool, queueWait time.Duration) error {
	if len(rules) == 0 {
		configuredPhysicalPool.Store(nil)
		return nil
	}
	if queueWait < time.Second || queueWait > 24*time.Hour {
		return fmt.Errorf("physical model queue wait timeout is invalid")
	}
	if err := agent_run.ValidateProductionRedis(ctx, client); err != nil {
		return err
	}
	pool, err := agent_run.NewRedisModelPool(client)
	if err != nil {
		return err
	}
	byTarget := make(map[string]agentconfig.PhysicalModelPool, len(rules))
	byPool := make(map[string]int, len(rules))
	for _, rule := range rules {
		key := modelTargetKey(rule.Provider, rule.Endpoint, rule.Model)
		if strings.TrimSpace(rule.PoolID) == "" || strings.TrimSpace(rule.Provider) == "" ||
			strings.TrimSpace(rule.Endpoint) == "" || strings.TrimSpace(rule.Model) == "" ||
			rule.Capacity < 1 || rule.Capacity > 10000 || rule.MaxWaiting < 1 || rule.MaxWaiting > 100000 ||
			rule.LeaseTTL < RequestTimeout()+10*time.Second || rule.LeaseTTL > time.Hour {
			return fmt.Errorf("physical model pool rule is invalid for %q", rule.PoolID)
		}
		if _, duplicate := byTarget[key]; duplicate {
			return fmt.Errorf("duplicate physical model target %q", key)
		}
		if capacity, used := byPool[rule.PoolID]; used && capacity != rule.Capacity {
			return fmt.Errorf("physical model pool %q has conflicting capacities", rule.PoolID)
		}
		byPool[rule.PoolID] = rule.Capacity
		byTarget[key] = rule
	}
	configuredPhysicalPool.Store(&physicalPoolRuntime{pool: pool, rules: byTarget, queueWait: queueWait})
	return nil
}

func modelTargetKey(provider, endpoint, model string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if provider == "ollama" && endpoint == "" {
		endpoint = "http://127.0.0.1:11434"
	}
	return provider + "\x00" + endpoint + "\x00" + strings.TrimSpace(model)
}

// acquireModelCallLease applies the tenant Profile limit first, then waits
// for one physical deployment slot. The provider request timeout starts only
// after this function returns. The returned completion function is idempotent.
func acquireModelCallLease(ctx context.Context, mc *modelconfig.ModelConfig) (context.Context, func() error, error) {
	profileRelease, err := acquireModelConcurrencyLease(ctx, mc)
	if err != nil {
		return nil, nil, err
	}
	runtime := configuredPhysicalPool.Load()
	if runtime == nil {
		return ctx, func() error { profileRelease(); return nil }, nil
	}
	if mc == nil {
		profileRelease()
		return nil, nil, fmt.Errorf("model config is required")
	}
	rule, found := runtime.rules[modelTargetKey(mc.Provider, mc.Endpoint, mc.Model)]
	if !found {
		if strings.EqualFold(strings.TrimSpace(mc.Provider), "ollama") {
			profileRelease()
			return nil, nil, ErrPhysicalModelPoolUnconfigured
		}
		return ctx, func() error { profileRelease(); return nil }, nil
	}
	waitCtx, waitCancel := context.WithTimeout(ctx, runtime.queueWait)
	owner := firstNonEmpty(strings.TrimSpace(reqctx.GetTenantUUID(ctx)), "global") + ":" +
		firstNonEmpty(strings.TrimSpace(reqctx.GetEnv(ctx)), "default")
	lease, err := runtime.pool.WaitAcquire(waitCtx, rule.PoolID, owner, rule.Capacity, rule.MaxWaiting, rule.LeaseTTL)
	waitTimedOut := errors.Is(waitCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil
	waitCancel()
	if err != nil {
		profileRelease()
		if waitTimedOut {
			return nil, nil, fmt.Errorf("%w: %v", ErrModelQueueTimeout, err)
		}
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		return nil, nil, errors.Join(ErrPhysicalModelPoolUnavailable, err)
	}
	callCtx, cancelCall := context.WithCancel(ctx)
	heartbeatDone := make(chan error, 1)
	go func() {
		interval := rule.LeaseTTL / 3
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-callCtx.Done():
				heartbeatDone <- nil
				return
			case <-ticker.C:
				renewCtx, cancel := context.WithTimeout(callCtx, interval)
				_, renewErr := runtime.pool.Renew(renewCtx, lease, rule.LeaseTTL)
				cancel()
				if renewErr != nil {
					cancelCall()
					heartbeatDone <- renewErr
					return
				}
			}
		}
	}()
	var once sync.Once
	var closeErr error
	finish := func() error {
		once.Do(func() {
			cancelCall()
			closeErr = <-heartbeatDone
			releaseCtx, releaseCancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer releaseCancel()
			if err := runtime.pool.Release(releaseCtx, lease); err != nil && closeErr == nil {
				closeErr = err
			}
			profileRelease()
			if closeErr != nil {
				closeErr = errors.Join(ErrPhysicalModelPoolUnavailable, closeErr)
			}
		})
		return closeErr
	}
	return callCtx, finish, nil
}
