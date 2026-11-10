package shared

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRedisOptionsScopeAllConfiguredPrefixesWithoutMutatingInput(t *testing.T) {
	input := &DepsOptions{EnvironmentID: "existing-instance-a"}
	a := scopedRedisOptions(input)
	b := scopedRedisOptions(input)
	require.Equal(t, "", input.EventFabric.Authorization.CacheInvalidateChannel)
	require.Equal(t, a.EventFabric.Authorization.CacheInvalidateChannel, b.EventFabric.Authorization.CacheInvalidateChannel)
	input.EnvironmentID = "existing-instance-b"
	c := scopedRedisOptions(input)
	require.NotEqual(t, a.EventFabric.Authorization.CacheInvalidateChannel, c.EventFabric.Authorization.CacheInvalidateChannel)
	require.NotEqual(t, a.EventFabric.Authorization.RateLimitPrefix, c.EventFabric.Authorization.RateLimitPrefix)
	require.NotEqual(t, a.IntegrationGateway.RateLimitPrefix, c.IntegrationGateway.RateLimitPrefix)
	require.NotEqual(t, a.AgentLifecycle.CapacityKeyPrefix, c.AgentLifecycle.CapacityKeyPrefix)
	require.NotEqual(t, a.KnowledgeSpace.LockKeyPrefix, c.KnowledgeSpace.LockKeyPrefix)
}
