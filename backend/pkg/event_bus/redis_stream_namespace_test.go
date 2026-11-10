package event_bus

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestStreamNamespacesIsolateDeliveryDedupeDelayAndFencing(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	prod, err := NewRedisStreamTaskDriver(client, time.Second, "prod:primary")
	require.NoError(t, err)
	dev, err := NewRedisStreamTaskDriver(client, time.Second, "dev:primary")
	require.NoError(t, err)
	msg := TaskMessage{ID: "same-original", TenantKey: "core-media", SubscriberID: "image-derivatives-v2", Topic: "media.image-derivatives.changed", Payload: []byte(`{}`)}
	require.NoError(t, prod.Enqueue(ctx, msg))
	empty, err := dev.Dequeue(ctx, msg.TenantKey, msg.SubscriberID, "workers", "same-worker", 1, time.Millisecond)
	require.NoError(t, err)
	require.Empty(t, empty)
	require.NoError(t, dev.Enqueue(ctx, msg))
	require.NoError(t, dev.Enqueue(ctx, msg))
	a, err := prod.Dequeue(ctx, msg.TenantKey, msg.SubscriberID, "workers", "same-worker", 1, time.Millisecond)
	require.NoError(t, err)
	require.Len(t, a, 1)
	b, err := dev.Dequeue(ctx, msg.TenantKey, msg.SubscriberID, "workers", "same-worker", 1, time.Millisecond)
	require.NoError(t, err)
	require.Len(t, b, 1)
	require.ErrorIs(t, prod.ValidateLease(ctx, b[0].Lease), ErrLeaseStale)
	require.Error(t, prod.Ack(ctx, b[0]))
	require.NoError(t, prod.ValidateLease(ctx, a[0].Lease))
	_, err = prod.Nack(ctx, a[0], time.Now().Add(time.Hour), "transient", 3)
	require.NoError(t, err)
	require.NoError(t, dev.Ack(ctx, b[0]))
	mini.FastForward(2 * time.Hour)
	// Promotion uses wall-clock, so explicitly expose the delayed record as due.
	delay := strings.TrimSuffix(prod.streamKey(msg.TenantKey, msg.SubscriberID), ":stream") + ":delay"
	rows, err := client.ZRange(ctx, delay, 0, -1).Result()
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NoError(t, client.ZAdd(ctx, delay, redis.Z{Score: 0, Member: rows[0]}).Err())
	n, err := dev.PromoteDue(ctx, msg.TenantKey, msg.SubscriberID, 10)
	require.NoError(t, err)
	require.Zero(t, n)
	n, err = prod.PromoteDue(ctx, msg.TenantKey, msg.SubscriberID, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
}
