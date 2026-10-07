package agent_run

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestExecutionCapacityProcessHelper(t *testing.T) {
	raw := os.Getenv("POWERX_EXECUTION_TEST_SCOPE")
	if raw == "" {
		t.Skip("subprocess helper")
	}
	var id Snapshot
	require.NoError(t, json.Unmarshal([]byte(raw), &id))
	client := redis.NewClient(&redis.Options{Addr: os.Getenv("POWERX_AGENT_RUN_TEST_REDIS_ADDR")})
	defer client.Close()
	gate, err := NewExecutionCapacity(context.Background(), client, id.Env, ExecutionCapacityPolicy{TenantLimit: 2, RunLimit: 1, LeaseTTL: 10 * time.Second})
	require.NoError(t, err)
	lease, err := gate.Acquire(context.Background(), id)
	if err != nil {
		fmt.Println("CAPACITY_RESULT=" + err.Error())
		return
	}
	require.NoError(t, gate.Release(context.Background(), lease))
	fmt.Println("CAPACITY_RESULT=acquired")
}

func TestExecutionCapacityAcrossProcessesRealRedis(t *testing.T) {
	address := os.Getenv("POWERX_AGENT_RUN_TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("set POWERX_AGENT_RUN_TEST_REDIS_ADDR to an AOF development Redis")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client := redis.NewClient(&redis.Options{Addr: address})
	t.Cleanup(func() { _ = client.Close() })
	require.NoError(t, ValidateProductionRedis(ctx, client))
	id := capacityIdentity()
	id.Env = "test-" + uuid.NewString()[:16]
	gate, err := NewExecutionCapacity(ctx, client, id.Env, ExecutionCapacityPolicy{TenantLimit: 2, RunLimit: 1, LeaseTTL: 10 * time.Second})
	require.NoError(t, err)
	another := id
	another.RunID = uuid.NewString()
	third := id
	third.RunID = uuid.NewString()
	otherTenant := id
	otherTenant.TenantUUID = uuid.NewString()
	otherTenant.RunID = uuid.NewString()
	t.Cleanup(func() {
		keys := []string{"agent:scheduling:{" + id.Env + "}:policy"}
		for _, scope := range []Snapshot{id, another, third, otherTenant} {
			keys = append(keys, executionCapacityKeys(scope)...)
		}
		_ = client.Del(context.Background(), keys...).Err()
	})
	runChild := func(scope Snapshot, expected string) {
		raw, err := json.Marshal(scope)
		require.NoError(t, err)
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestExecutionCapacityProcessHelper$", "-test.count=1")
		cmd.Env = append(os.Environ(), "POWERX_EXECUTION_TEST_SCOPE="+string(raw))
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		require.True(t, strings.Contains(string(out), "CAPACITY_RESULT="+expected), string(out))
	}
	held, err := gate.Acquire(ctx, id)
	require.NoError(t, err)
	runChild(id, "capacity.run")
	second, err := gate.Acquire(ctx, another)
	require.NoError(t, err)
	runChild(third, "capacity.tenant")
	runChild(otherTenant, "acquired")
	require.NoError(t, gate.Release(ctx, held))
	runChild(id, "acquired")
	require.NoError(t, gate.Release(ctx, second))
}
