package event_bus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewRedisTaskDriverDefaults(t *testing.T) {
	t.Parallel()

	driver := NewRedisTaskDriver(RedisTaskDriverOptions{})
	impl, ok := driver.(*redisTaskDriver)
	require.True(t, ok)
	require.Equal(t, defaultTaskQueuePrefix, impl.prefix)
	require.Equal(t, defaultTaskDequeueTimeout, impl.blockingTimeout)
	require.Equal(t, defaultTaskProcessingExpiry, impl.processingExpiry)

	capability := driver.Capability()
	require.True(t, capability.SupportsBlockingDequeue)
	require.True(t, capability.SupportsDelayQueue)
	require.False(t, capability.SupportsLease)
	require.False(t, capability.SupportsConsumerGroup)
}

func TestRedisTaskDriverValidateNoClient(t *testing.T) {
	t.Parallel()

	driver := NewRedisTaskDriver(RedisTaskDriverOptions{})
	err := driver.Enqueue(context.Background(), TaskMessage{ID: "m1", TenantKey: "global", SubscriberID: "worker"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "redis client is required")
}

func TestValidateTaskMessage(t *testing.T) {
	t.Parallel()

	require.Error(t, validateTaskMessage(TaskMessage{}))
	require.Error(t, validateTaskMessage(TaskMessage{ID: "m1"}))
	require.Error(t, validateTaskMessage(TaskMessage{ID: "m1", TenantKey: "global"}))
	require.NoError(t, validateTaskMessage(TaskMessage{ID: "m1", TenantKey: "global", SubscriberID: "worker"}))
}

func TestRedisTaskDriverRetryUsesRetryAt(t *testing.T) {
	t.Parallel()

	driver := NewRedisTaskDriver(RedisTaskDriverOptions{})
	retryAt := time.Now().Add(time.Minute)
	err := driver.Retry(context.Background(), RetryRequest{
		Message: TaskMessage{ID: "m1", TenantKey: "global", SubscriberID: "worker"},
		RetryAt: retryAt,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "redis client is required")
}

