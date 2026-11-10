package shared

import "github.com/ArtisanCloud/PowerX/pkg/redisnamespace"

// scopedRedisOptions copies deployment options before applying the installation
// scope, so constructing another dependency container never doubles prefixes.
func scopedRedisOptions(opts *DepsOptions) *DepsOptions {
	c := *opts
	c.EventFabric.EnvironmentID = c.EnvironmentID
	key := func(value, fallback string) string {
		if value == "" {
			value = fallback
		}
		return redisnamespace.Key(c.EnvironmentID, value)
	}
	c.EventFabric.RetryKeyPrefix = key(c.EventFabric.RetryKeyPrefix, "event_fabric:retry")
	c.EventFabric.ReplayKeyPrefix = key(c.EventFabric.ReplayKeyPrefix, "event_fabric:replay")
	c.EventFabric.Authorization.CacheInvalidateChannel = key(c.EventFabric.Authorization.CacheInvalidateChannel, "event_fabric:authorization:invalidate")
	c.EventFabric.Authorization.RateLimitPrefix = key(c.EventFabric.Authorization.RateLimitPrefix, "event_fabric:authorization:rl")
	c.IntegrationGateway.RateLimitPrefix = key(c.IntegrationGateway.RateLimitPrefix, "integration_gateway:rl")
	c.AgentLifecycle.CapacityKeyPrefix = key(c.AgentLifecycle.CapacityKeyPrefix, "agent_lifecycle:capacity")
	c.AgentLifecycle.HealthKeyPrefix = key(c.AgentLifecycle.HealthKeyPrefix, "agent_lifecycle:health")
	c.KnowledgeSpace.LockKeyPrefix = key(c.KnowledgeSpace.LockKeyPrefix, "knowledge_space:lock")
	c.KnowledgeSpace.MetricsKeyPrefix = key(c.KnowledgeSpace.MetricsKeyPrefix, "knowledge_space:metrics")
	return &c
}
