package event_bus

import (
	"context"
	"errors"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestCapacityDeferDoesNotConsumeRetriesOrPermitStaleAck(t *testing.T) {
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { client.Close() })
	queue, err := NewRedisStreamTaskDriver(client, time.Second)
	require.NoError(t, err)
	message := TaskMessage{ID: "capacity-run:1:task:1", TenantKey: "capacity-tenant", SubscriberID: "agent-run", Topic: "agent.task", Attempt: 99}
	require.NoError(t, queue.Enqueue(ctx, message))
	var previous StreamTaskDelivery
	for i := 0; i < 105; i++ {
		rows, err := queue.Dequeue(ctx, message.TenantKey, message.SubscriberID, "workers", "worker", 1, time.Millisecond)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, 99, rows[0].Message.Attempt)
		if i > 0 {
			require.True(t, rows[0].Lease.Token > previous.Lease.Token)
			require.True(t, errors.Is(queue.Ack(ctx, previous), ErrLeaseStale))
		}
		previous = rows[0]
		require.NoError(t, queue.Defer(ctx, rows[0], time.Now().Add(-time.Second), "capacity.run"))
	}
	outcome, err := queue.Nack(ctx, previous, time.Now(), "failed", 100)
	require.ErrorIs(t, err, ErrLeaseStale)
	require.Empty(t, outcome)
	rows, err := queue.Dequeue(ctx, message.TenantKey, message.SubscriberID, "workers", "worker", 1, time.Millisecond)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	outcome, err = queue.Nack(ctx, rows[0], time.Now(), "real_failure", 100)
	require.NoError(t, err)
	require.Equal(t, StreamDead, outcome)
}
