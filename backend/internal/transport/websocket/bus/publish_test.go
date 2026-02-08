package bus

import "testing"

func resetPublishRegistry() {
	publishDynamicTopics.mu.Lock()
	publishDynamicTopics.byTenant = make(map[string]map[string]struct{})
	publishDynamicTopics.mu.Unlock()
}

func TestRegisterPublishTopicsAllowsTenantScopedTopics(t *testing.T) {
	resetPublishRegistry()

	tenantA := "tenant-a"
	tenantB := "tenant-b"
	topic := "custom.progress"

	if IsPublishTopicAllowed(tenantA, topic) {
		t.Fatalf("expected topic to be disallowed before register")
	}

	registered := RegisterPublishTopics(tenantA, []string{topic, topic, " "})
	if len(registered) != 1 {
		t.Fatalf("expected one registered topic, got %d", len(registered))
	}

	if !IsPublishTopicAllowed(tenantA, topic) {
		t.Fatalf("expected topic to be allowed after register for tenant-a")
	}
	if IsPublishTopicAllowed(tenantB, topic) {
		t.Fatalf("expected topic to be disallowed for tenant-b")
	}
}

func TestPublishAllowedTopicsIncludeStaticWhitelist(t *testing.T) {
	resetPublishRegistry()

	if !IsPublishTopicAllowed("tenant-a", TopicOrgSyncProgress) {
		t.Fatalf("expected static whitelist topic to be allowed")
	}
	if !IsPublishTopicAllowed("tenant-a", TopicOrgSyncProgressV1) {
		t.Fatalf("expected static whitelist topic v1 to be allowed")
	}
}
