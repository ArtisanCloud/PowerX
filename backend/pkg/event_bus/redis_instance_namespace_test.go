package event_bus

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestReliableQueueNamespacesIsolateRetriesAndLeasesInSameDB(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	a := NewRedisReliableQueue(client, "dev:instance-a")
	b := NewRedisReliableQueue(client, "dev:instance-b")
	item := RetryItem{TenantKey: "same-tenant", ExecuteAt: time.Now().Add(-time.Second)}
	require.NoError(t, a.ScheduleRetry(ctx, item))
	empty, err := b.PopDueRetries(ctx, item.TenantKey, time.Now(), 10)
	require.NoError(t, err)
	require.Empty(t, empty)
	items, err := a.PopDueRetries(ctx, item.TenantKey, time.Now(), 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	lease := DeliveryLease{TenantKey: "same-tenant", SubscriberID: "same-subscriber", LeaseID: "same-lease", MaxConcurrent: 1}
	one, err := a.AcquireLease(ctx, lease)
	require.NoError(t, err)
	two, err := b.AcquireLease(ctx, lease)
	require.NoError(t, err)
	require.True(t, one)
	require.True(t, two)
}

func TestEventBusNamespacesIsolateChannelsEvenAcrossRedisDBs(t *testing.T) {
	mini := miniredis.RunT(t)
	a := NewRedisEventBus(&redis.Options{Addr: mini.Addr(), DB: 0}, "dev:instance-a")
	b := NewRedisEventBus(&redis.Options{Addr: mini.Addr(), DB: 1}, "dev:instance-b")
	t.Cleanup(func() { _ = a.Close(); _ = b.Close() })
	one, two := make(chan struct{}, 1), make(chan struct{}, 1)
	a.Subscribe("same-event", func(Event) error { one <- struct{}{}; return nil })
	b.Subscribe("same-event", func(Event) error { two <- struct{}{}; return nil })
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	require.Eventually(t, func() bool { return client.PubSubNumPat(context.Background()).Val() == 2 }, time.Second, time.Millisecond)
	a.Publish("same-event", map[string]any{"ok": true}, context.Background())
	select {
	case <-one:
	case <-time.After(time.Second):
		t.Fatal("own event was not delivered")
	}
	select {
	case <-two:
		t.Fatal("event crossed installation namespace")
	case <-time.After(30 * time.Millisecond):
	}
}
