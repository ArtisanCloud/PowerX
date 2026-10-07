package agent_run

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func capacityIdentity() Snapshot {
	return Snapshot{TenantUUID: uuid.NewString(), Env: "test", RunID: uuid.NewString()}
}
func TestExecutionCapacitySharedAcrossClientsAndRuns(t *testing.T) {
	ctx := context.Background()
	server := miniredis.RunT(t)
	a := redis.NewClient(&redis.Options{Addr: server.Addr()})
	b := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { a.Close(); b.Close() })
	policy := ExecutionCapacityPolicy{TenantLimit: 2, RunLimit: 1, LeaseTTL: time.Second}
	first, err := NewExecutionCapacity(ctx, a, "test", policy)
	require.NoError(t, err)
	second, err := NewExecutionCapacity(ctx, b, "test", policy)
	require.NoError(t, err)
	id := capacityIdentity()
	anotherRun := id
	anotherRun.RunID = uuid.NewString()
	var winners atomic.Int32
	var wg sync.WaitGroup
	leases := make(chan ExecutionLease, 50)
	failures := make(chan error, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			gate := first
			if i%2 == 0 {
				gate = second
			}
			lease, e := gate.Acquire(ctx, id)
			if e == nil {
				winners.Add(1)
				leases <- lease
			} else {
				failures <- e
			}
		}(i)
	}
	wg.Wait()
	require.EqualValues(t, 1, winners.Load())
	close(failures)
	for e := range failures {
		require.ErrorIs(t, e, ErrRunCapacity)
	}
	lease2, err := second.Acquire(ctx, anotherRun)
	require.NoError(t, err)
	thirdRun := id
	thirdRun.RunID = uuid.NewString()
	_, err = first.Acquire(ctx, thirdRun)
	require.ErrorIs(t, err, ErrTenantCapacity)
	otherTenant := capacityIdentity()
	otherLease, err := second.Acquire(ctx, otherTenant)
	require.NoError(t, err)
	require.NoError(t, first.Release(ctx, <-leases))
	require.NoError(t, second.Release(ctx, lease2))
	require.NoError(t, second.Release(ctx, otherLease))
	recovered, err := second.Acquire(ctx, id)
	require.NoError(t, err)
	require.NoError(t, second.Release(ctx, recovered))
	require.NoError(t, second.Release(ctx, recovered))
	_, err = NewExecutionCapacity(ctx, b, "test", ExecutionCapacityPolicy{TenantLimit: 3, RunLimit: 1, LeaseTTL: time.Second})
	require.ErrorIs(t, err, ErrSchedulingPolicyDrift)
	wrongEnv := id
	wrongEnv.Env = "dev"
	_, err = first.Acquire(ctx, wrongEnv)
	require.ErrorIs(t, err, ErrInvalid)
}

func TestExecutionCapacityExpiryCannotBeRenewedOrReleaseNewOwner(t *testing.T) {
	ctx := context.Background()
	server := miniredis.RunT(t)
	now := time.Now()
	server.SetTime(now)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { client.Close() })
	gate, err := NewExecutionCapacity(ctx, client, "test", ExecutionCapacityPolicy{TenantLimit: 1, RunLimit: 1, LeaseTTL: time.Second})
	require.NoError(t, err)
	id := capacityIdentity()
	old, err := gate.Acquire(ctx, id)
	require.NoError(t, err)
	server.SetTime(now.Add(1100 * time.Millisecond))
	require.ErrorIs(t, gate.Renew(ctx, old), ErrExecutionLeaseStale)
	next, err := gate.Acquire(ctx, id)
	require.NoError(t, err)
	require.NoError(t, gate.Release(ctx, old))
	require.NoError(t, gate.Renew(ctx, next))
	_, err = gate.Acquire(ctx, id)
	require.ErrorIs(t, err, ErrRunCapacity)
	require.NoError(t, gate.Release(ctx, next))
	server.FastForward(3 * time.Second)
	require.False(t, server.Exists(executionCapacityKeys(id)[0]))
	require.False(t, server.Exists(executionCapacityKeys(id)[1]))
	require.False(t, server.Exists(executionCapacityKeys(id)[2]))
}

func TestExecutionCapacityLostRenewalCancelsExecution(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { client.Close() })
	gate, err := NewExecutionCapacity(ctx, client, "test", ExecutionCapacityPolicy{TenantLimit: 1, RunLimit: 1, LeaseTTL: time.Second})
	require.NoError(t, err)
	id := capacityIdentity()
	lease, err := gate.Acquire(ctx, id)
	require.NoError(t, err)
	result := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		result <- gate.Execute(ctx, lease, func(runCtx context.Context) error { close(started); <-runCtx.Done(); return runCtx.Err() })
	}()
	<-started
	require.NoError(t, client.ZRem(ctx, executionCapacityKeys(id)[0], lease.Token).Err())
	select {
	case err := <-result:
		require.True(t, errors.Is(err, context.Canceled))
		require.ErrorIs(t, err, ErrExecutionLeaseStale)
	case <-ctx.Done():
		t.Fatal("lost capacity lease did not cancel execution")
	}
	next, err := gate.Acquire(ctx, id)
	require.NoError(t, err)
	require.NoError(t, gate.Release(ctx, next))
}
