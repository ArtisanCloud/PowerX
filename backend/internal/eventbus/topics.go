package eventbus

// Canonical event topics shared by Integration Gateway, Capability Registry and Agent Hub.
// 按 spec requirement（FR-005/FR-011）统一罗列，便于事件生产者/消费者引用。
const (
	// Integration Gateway lifecycle topics.
	TopicIntegrationGatewayRouteCreated = "integration.gateway.route.created"
	TopicIntegrationGatewayRouteUpdated = "integration.gateway.route.updated"

	// Integration Gateway invocation topics.
	TopicIntegrationGatewayInvocationSucceeded = "integration.gateway.invocation.succeeded"
	TopicIntegrationGatewayInvocationFailed    = "integration.gateway.invocation.failed"
	TopicIntegrationGatewayInvocationFallback  = "integration.gateway.invocation.fallback"

	// Capability Catalog sync topics.
	TopicCapabilityCatalogSyncStarted   = "capability.catalog.sync_started"
	TopicCapabilityCatalogSyncSucceeded = "capability.catalog.sync_succeeded"
	TopicCapabilityCatalogSyncFailed    = "capability.catalog.sync_failed"

	// Capability policy governance topics.
	TopicCapabilityPolicyDegraded = "capability.policy.degraded"
)
