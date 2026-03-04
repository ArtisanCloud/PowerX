package eventbus

const (
	TopicKnowledgeIngestionJob      = "_topic.knowledge.ingestion.job"
	TopicKnowledgeCorpusCheckJob    = "_topic.knowledge.corpus_check.job"
	TopicKnowledgeFeedbackReprocess = "_topic.knowledge.space.feedback.reprocess"
	TopicSystemNotification         = "_topic.system.notification"

	TopicIntegrationGatewayRouteCreated = "_topic.integration.gateway.route.created"
	TopicIntegrationGatewayRouteUpdated = "_topic.integration.gateway.route.updated"

	TopicIntegrationGatewayInvocationSucceeded = "_topic.integration.gateway.invocation.succeeded"
	TopicIntegrationGatewayInvocationFailed    = "_topic.integration.gateway.invocation.failed"
	TopicIntegrationGatewayInvocationFallback  = "_topic.integration.gateway.invocation.fallback"

	TopicCapabilityCatalogSyncStarted   = "_topic.capability.catalog.sync_started"
	TopicCapabilityCatalogSyncSucceeded = "_topic.capability.catalog.sync_succeeded"
	TopicCapabilityCatalogSyncFailed    = "_topic.capability.catalog.sync_failed"

	TopicCapabilityPolicyDegraded = "_topic.capability.policy.degraded"

	NotificationKindEventFabricReplayTask = "_kind.event_fabric.replay.task"
)
