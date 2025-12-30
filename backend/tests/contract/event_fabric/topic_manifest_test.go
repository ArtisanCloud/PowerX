package eventfabric_test

import (
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/manifest"
)

const sampleManifest = `
version: 1
defaults:
  payload_format: json
  versioning_mode: backward
  max_retry: 5
  ack_timeout_seconds: 45
  retention_policy:
    type: time
    value: 7d
  metadata:
    origin: platform
    channel: default
topics:
  - key: orders.created
    namespace: orders
    name: created
    ack_timeout_seconds: 60
    acl:
      - principal_type: service
        principal_id: "{{ plugin_id }}-writer"
        actions: [publish]
      - principal_type: service
        principal_id: "{{ tenant_uuid }}-consumer"
        actions: [subscribe, replay]
  - namespace: orders
    name: cancelled
    payload_format: protobuf
    max_retry: 2
    acl:
      - principal_type: service
        principal_id: "ops-{{ variables.cluster }}"
        actions: [publish]
`

func TestManifestRenderSuccess(t *testing.T) {
	m, err := manifest.Parse(strings.NewReader(sampleManifest))
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}

	plan, err := m.Render(manifest.SeedContext{
		TenantUUID:    "aeffc79f-e72a-4fd9-b908-5c150bce3741",
		PluginID:      "demo-plugin",
		PluginVersion: "1.2.3",
		Operator:      "system",
		Variables: map[string]string{
			"cluster": "east",
		},
	})
	if err != nil {
		t.Fatalf("render manifest: %v", err)
	}

	if len(plan.Topics) != 2 {
		t.Fatalf("expected 2 topics, got %d", len(plan.Topics))
	}

	first := plan.Topics[0]
	if first.FullTopic != "aeffc79f-e72a-4fd9-b908-5c150bce3741.orders.created" {
		t.Fatalf("unexpected full topic: %s", first.FullTopic)
	}
	if first.Topic.PayloadFormat != "json" {
		t.Fatalf("payload format mismatch: %s", first.Topic.PayloadFormat)
	}
	if first.Topic.AckTimeoutSec != 60 {
		t.Fatalf("ack timeout mismatch: %d", first.Topic.AckTimeoutSec)
	}
	if len(first.ACL) != 2 {
		t.Fatalf("expected 2 acl entries, got %d", len(first.ACL))
	}
	if first.ACL[0].PrincipalID != "demo-plugin-writer" {
		t.Fatalf("principal templating failed: %s", first.ACL[0].PrincipalID)
	}
	if first.ACL[1].PrincipalID != "aeffc79f-e72a-4fd9-b908-5c150bce3741-consumer" {
		t.Fatalf("tenant templating failed: %s", first.ACL[1].PrincipalID)
	}

	second := plan.Topics[1]
	if second.Topic.PayloadFormat != "protobuf" {
		t.Fatalf("expected protobuf payload format, got %s", second.Topic.PayloadFormat)
	}
	if second.Topic.MaxRetry != 2 {
		t.Fatalf("expected max_retry=2, got %d", second.Topic.MaxRetry)
	}
	if len(second.ACL) != 1 || second.ACL[0].PrincipalID != "ops-east" {
		t.Fatalf("cluster variable substitution failed: %+v", second.ACL)
	}
}

func TestManifestValidationErrors(t *testing.T) {
	bad := `
version: 1
topics:
  - namespace: ""
    name: foo
`
	if _, err := manifest.Parse(strings.NewReader(bad)); err == nil {
		t.Fatalf("expected error for empty namespace")
	}

	badAction := `
version: 1
topics:
  - namespace: foo
    name: bar
    acl:
      - principal_type: service
        principal_id: svc
        actions: [publish, unknown]
`
	m, err := manifest.Parse(strings.NewReader(badAction))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if _, err := m.Render(manifest.SeedContext{TenantUUID: "tenant"}); err == nil {
		t.Fatalf("expected render error for invalid action")
	}
}
