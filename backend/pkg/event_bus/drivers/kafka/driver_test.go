package kafka

import (
	"context"
	"errors"
	"testing"
	"time"

	eventbus "github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/stretchr/testify/require"
)

func TestNewDriverDefaults(t *testing.T) {
	t.Parallel()

	d := NewDriver(DriverOptions{}).(*driver)
	require.Equal(t, "event_fabric.task", d.topicPrefix)
	require.Equal(t, "powerx.event_fabric", d.consumerGroup)
	require.Equal(t, time.Second, d.pollTimeout)
	require.Empty(t, d.brokers)

	capability := d.Capability()
	require.True(t, capability.SupportsBlockingDequeue)
	require.True(t, capability.SupportsDelayQueue)
	require.True(t, capability.SupportsConsumerGroup)
}

func TestDequeueTriggersRebalanceAssigned(t *testing.T) {
	t.Parallel()

	assigned := false
	fallback := &stubTaskDriver{}
	d := NewDriver(DriverOptions{
		FallbackDriver: fallback,
		Rebalance: RebalanceHandlerFunc{
			Assigned: func(_ context.Context, session ConsumerGroupSession) {
				assigned = true
				require.Equal(t, "tenant:worker", session.MemberID)
			},
		},
	})

	_, err := d.Dequeue(context.Background(), eventbus.DequeueRequest{
		TenantKey:    "tenant",
		SubscriberID: "worker",
		MaxItems:     1,
		WaitTimeout:  10 * time.Millisecond,
	})
	require.NoError(t, err)
	require.True(t, assigned)
}

func TestAckCommitsOffsetAndFallback(t *testing.T) {
	t.Parallel()

	fallback := &stubTaskDriver{}
	d := NewDriver(DriverOptions{FallbackDriver: fallback}).(*driver)
	_, _ = d.Dequeue(context.Background(), eventbus.DequeueRequest{TenantKey: "tenant", SubscriberID: "worker"})

	err := d.Ack(context.Background(), eventbus.AckRequest{TenantKey: "tenant", SubscriberID: "worker", MessageID: "m1"})
	require.NoError(t, err)
	require.Equal(t, 1, fallback.ackCalls)

	commits := d.CommitLog()
	require.Len(t, commits, 1)
	require.Equal(t, "ack", commits[0].Metadata)
	require.Contains(t, commits[0].Topic, "event_fabric.task.tenant.worker")
}

func TestFallbackMissingReturnsError(t *testing.T) {
	t.Parallel()

	d := NewDriver(DriverOptions{})
	err := d.Enqueue(context.Background(), eventbus.TaskMessage{ID: "m1", TenantKey: "tenant", SubscriberID: "worker"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not wired with broker adapter")
}

type stubTaskDriver struct {
	ackCalls int
}

func (s *stubTaskDriver) Type() eventbus.QueueDriverType { return eventbus.QueueDriverMemory }

func (s *stubTaskDriver) Capability() eventbus.QueueDriverCapability { return eventbus.QueueDriverCapability{} }

func (s *stubTaskDriver) Enqueue(context.Context, eventbus.TaskMessage) error { return nil }

func (s *stubTaskDriver) Dequeue(context.Context, eventbus.DequeueRequest) ([]eventbus.TaskMessage, error) {
	return []eventbus.TaskMessage{{ID: "m1", TenantKey: "tenant", SubscriberID: "worker"}}, nil
}

func (s *stubTaskDriver) Ack(context.Context, eventbus.AckRequest) error {
	s.ackCalls++
	return nil
}

func (s *stubTaskDriver) Nack(context.Context, eventbus.NackRequest) error { return nil }

func (s *stubTaskDriver) Retry(context.Context, eventbus.RetryRequest) error { return errors.New("not used") }
