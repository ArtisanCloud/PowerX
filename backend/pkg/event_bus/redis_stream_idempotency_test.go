package event_bus

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestStreamTaskIdempotencyPreservesImmutableEnvelope(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	driver, err := NewRedisStreamTaskDriver(client, time.Second, "fixture_env")
	require.NoError(t, err)
	ctx := context.Background()
	message := TaskMessage{ID: "fixture_id", IdempotencyKey: "fixture_key", TenantKey: "fixture_tenant", SubscriberID: "fixture_source_subscriber", Topic: "fixture_topic", Payload: []byte(`{"value":1}`)}
	require.NoError(t, driver.Enqueue(ctx, message))
	require.NoError(t, driver.Enqueue(ctx, message))
	for _, change := range []func(*TaskMessage){
		func(m *TaskMessage) { m.Payload = []byte(`{"value":2}`) },
		func(m *TaskMessage) { m.ID = "fixture_changed_id" },
		func(m *TaskMessage) { m.IdempotencyKey = "fixture_changed_key" },
		func(m *TaskMessage) { m.Topic = "fixture_other_topic" },
	} {
		changed := message
		change(&changed)
		require.True(t, errors.Is(driver.Enqueue(ctx, changed), ErrTaskIdempotencyConflict))
	}
	count, err := client.XLen(ctx, driver.streamKey(message.TenantKey, message.SubscriberID)).Result()
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	message.SubscriberID = "fixture_other_source_subscriber"
	require.NoError(t, driver.Enqueue(ctx, message))
	other, err := NewRedisStreamTaskDriver(client, time.Second, "fixture_other_env")
	require.NoError(t, err)
	require.NoError(t, other.Enqueue(ctx, message))
}
