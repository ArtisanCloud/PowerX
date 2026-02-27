package bus

import (
	"testing"
)

func resetPublishRegistry() {
	SetDynamicTopicCompatEnabledForTest(false)
	publishDynamicTopics.mu.Lock()
	publishDynamicTopics.byTenant = make(map[string]map[string]struct{})
	publishDynamicTopics.mu.Unlock()
}

func TestRegisterPublishTopicsDefaultNoDynamicAllow(t *testing.T) {
	resetPublishRegistry()

	tenantA := "tenant-a"
	topic := "custom.progress"

	if IsPublishTopicAllowed(tenantA, topic) {
		t.Fatalf("expected topic to be disallowed before register")
	}

	registered := RegisterPublishTopics(tenantA, []string{topic, topic, " "})
	if len(registered) != 1 {
		t.Fatalf("expected one registered topic, got %d", len(registered))
	}

	if IsPublishTopicAllowed(tenantA, topic) {
		t.Fatalf("expected topic still disallowed when compat is disabled")
	}
}

func TestRegisterPublishTopicsAllowsTenantScopedTopicsWhenCompatEnabled(t *testing.T) {
	resetPublishRegistry()
	SetDynamicTopicCompatEnabledForTest(true)

	tenantA := "tenant-a"
	tenantB := "tenant-b"
	topic := "custom.progress"

	registered := RegisterPublishTopics(tenantA, []string{topic, topic, " "})
	if len(registered) != 1 {
		t.Fatalf("expected one registered topic, got %d", len(registered))
	}

	if !IsPublishTopicAllowed(tenantA, topic) {
		t.Fatalf("expected topic to be allowed for tenant-a with compat enabled")
	}
	if IsPublishTopicAllowed(tenantB, topic) {
		t.Fatalf("expected topic to be disallowed for tenant-b")
	}
}

func TestPublishAllowedTopicsHasNoStaticWhitelist(t *testing.T) {
	resetPublishRegistry()

	if IsPublishTopicAllowed("tenant-a", "powerx.org_sync.progress") {
		t.Fatalf("expected topic to be disallowed without dynamic register")
	}
}
