package event_bus

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestAcknowledgementDistinguishesUnroutedDispatchedAndClosed(t *testing.T) {
	bus := NewLocalEventBus()
	ack := bus.(AcknowledgedPublisher)
	receipt, err := ack.PublishAcknowledged("test", map[string]string{}, context.Background())
	require.NoError(t, err)
	require.Equal(t, "unrouted", receipt.Status)
	require.NotEmpty(t, receipt.EventID)
	entered := make(chan Event, 1)
	release := make(chan struct{})
	done := make(chan error, 1)
	unbind := bus.Subscribe("test", func(e Event) error { entered <- e; <-release; done <- e.Ctx.Err(); return nil })
	defer unbind()
	ctx, cancel := context.WithCancel(context.Background())
	receipt, err = ack.PublishAcknowledged("test", nil, ctx)
	require.NoError(t, err)
	require.Equal(t, "dispatched", receipt.Status)
	require.EqualValues(t, 1, receipt.Subscribers)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("handler not scheduled")
	}
	cancel()
	close(release)
	select {
	case err := <-done:
		require.NoError(t, err, "caller completion must not immediately cancel asynchronously scheduled delivery")
	case <-time.After(time.Second):
		t.Fatal("handler did not finish")
	}
	require.NoError(t, bus.Close())
	_, err = ack.PublishAcknowledged("test", nil, context.Background())
	require.Error(t, err)
}
