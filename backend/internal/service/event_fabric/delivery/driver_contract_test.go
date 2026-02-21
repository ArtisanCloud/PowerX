package delivery

import (
	"context"
	"errors"
	"testing"

	eventbus "github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/stretchr/testify/require"
)

func TestDriverFallbackPolicyNormalize(t *testing.T) {
	t.Parallel()

	policy := DriverFallbackPolicy{
		Primary:  eventbus.QueueDriverType(" Redis "),
		Fallback: []eventbus.QueueDriverType{"database", "Redis", "database", "  ", "nats"},
	}

	normalized := policy.Normalize()
	require.Equal(t, eventbus.QueueDriverRedis, normalized.Primary)
	require.Equal(t, []eventbus.QueueDriverType{eventbus.QueueDriverDatabase, eventbus.QueueDriverNATS}, normalized.Fallback)
}

func TestResolveDriverSelection(t *testing.T) {
	t.Parallel()

	drivers := map[eventbus.QueueDriverType]eventbus.TaskDriver{
		eventbus.QueueDriverRedis:    stubTaskDriver{driverType: eventbus.QueueDriverRedis},
		eventbus.QueueDriverDatabase: stubTaskDriver{driverType: eventbus.QueueDriverDatabase},
	}

	selection, err := ResolveDriverSelection(DriverFallbackPolicy{
		Primary:  eventbus.QueueDriverRedis,
		Fallback: []eventbus.QueueDriverType{eventbus.QueueDriverKafka, eventbus.QueueDriverDatabase},
	}, drivers)
	require.NoError(t, err)
	require.Equal(t, eventbus.QueueDriverRedis, selection.Primary)
	require.Equal(t, []eventbus.QueueDriverType{eventbus.QueueDriverDatabase}, selection.FallbackCandidates)
	require.Contains(t, selection.Available, eventbus.QueueDriverRedis)
}

func TestResolveDriverSelectionPrimaryUnavailable(t *testing.T) {
	t.Parallel()

	_, err := ResolveDriverSelection(DriverFallbackPolicy{
		Primary: eventbus.QueueDriverRedis,
	}, map[eventbus.QueueDriverType]eventbus.TaskDriver{
		eventbus.QueueDriverDatabase: stubTaskDriver{driverType: eventbus.QueueDriverDatabase},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "primary driver redis is unavailable")
}

func TestTryOnDriverFallback(t *testing.T) {
	t.Parallel()

	var called []eventbus.QueueDriverType
	drivers := map[eventbus.QueueDriverType]eventbus.TaskDriver{
		eventbus.QueueDriverRedis:    stubTaskDriver{driverType: eventbus.QueueDriverRedis},
		eventbus.QueueDriverDatabase: stubTaskDriver{driverType: eventbus.QueueDriverDatabase},
	}
	selection := DriverSelection{
		Primary:            eventbus.QueueDriverRedis,
		FallbackCandidates: []eventbus.QueueDriverType{eventbus.QueueDriverDatabase},
	}

	err := TryOnDriver(context.Background(), selection, drivers, func(_ context.Context, driver eventbus.TaskDriver) error {
		called = append(called, driver.Type())
		if driver.Type() == eventbus.QueueDriverRedis {
			return errors.New("redis unavailable")
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, []eventbus.QueueDriverType{eventbus.QueueDriverRedis, eventbus.QueueDriverDatabase}, called)
}

func TestTryOnDriverWithoutFallback(t *testing.T) {
	t.Parallel()

	drivers := map[eventbus.QueueDriverType]eventbus.TaskDriver{
		eventbus.QueueDriverRedis: stubTaskDriver{driverType: eventbus.QueueDriverRedis},
	}
	selection := DriverSelection{Primary: eventbus.QueueDriverRedis}
	err := TryOnDriver(context.Background(), selection, drivers, func(_ context.Context, _ eventbus.TaskDriver) error {
		return errors.New("primary down")
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "driver redis execution failed without fallback")
}

type stubTaskDriver struct {
	driverType eventbus.QueueDriverType
}

func (s stubTaskDriver) Type() eventbus.QueueDriverType {
	return s.driverType
}

func (s stubTaskDriver) Capability() eventbus.QueueDriverCapability {
	return eventbus.QueueDriverCapability{}
}

func (s stubTaskDriver) Enqueue(context.Context, eventbus.TaskMessage) error {
	return nil
}

func (s stubTaskDriver) Dequeue(context.Context, eventbus.DequeueRequest) ([]eventbus.TaskMessage, error) {
	return nil, nil
}

func (s stubTaskDriver) Ack(context.Context, eventbus.AckRequest) error {
	return nil
}

func (s stubTaskDriver) Nack(context.Context, eventbus.NackRequest) error {
	return nil
}

func (s stubTaskDriver) Retry(context.Context, eventbus.RetryRequest) error {
	return nil
}

