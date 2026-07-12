package manifest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadAcceptsNumericVersion(t *testing.T) {
	data := []byte(`
version: 1
topics:
  - namespace: _topic.system
    name: notification
`)
	doc, err := Load(data)
	require.NoError(t, err)
	require.Equal(t, ManifestVersion(1), doc.Version)
}

func TestLoadAcceptsPrefixedVersion(t *testing.T) {
	data := []byte(`
version: v1
topics:
  - namespace: _topic.system
    name: notification
`)
	doc, err := Load(data)
	require.NoError(t, err)
	require.Equal(t, ManifestVersion(1), doc.Version)
}

func TestLoadRejectsInvalidVersion(t *testing.T) {
	data := []byte(`
version: vX
topics:
  - namespace: _topic.system
    name: notification
`)
	_, err := Load(data)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid manifest version")
}

func TestLoadAcceptsLegacyTopicField(t *testing.T) {
	data := []byte(`
version: v1
topics:
  - topic: _topic.template.update
    acl:
      - actions: [publish]
`)
	doc, err := Load(data)
	require.NoError(t, err)
	require.Equal(t, 1, doc.TopicCount())
}

func TestRenderSkipsLegacyACLWithoutPrincipal(t *testing.T) {
	data := []byte(`
version: v1
topics:
  - topic: _topic.template.update
    acl:
      - actions: [publish]
`)
	doc, err := Load(data)
	require.NoError(t, err)
	plan, err := doc.Render(SeedContext{
		TenantUUID:    "6b5d0240-9920-46da-b707-88200e0f51ea",
		PluginID:      "com.powerx.plugins.base",
		PluginVersion: "0.1.1",
	})
	require.NoError(t, err)
	require.Len(t, plan.Topics, 1)
	require.Equal(t, "6b5d0240-9920-46da-b707-88200e0f51ea._topic.template.update", plan.Topics[0].FullTopic)
	require.Empty(t, plan.Topics[0].ACL)
}

func TestRenderSplitsDottedTopicNameIntoNamespaceAndName(t *testing.T) {
	data := []byte(`
version: v1
topics:
  - topic: powerx.runtime.scheduler.triggered.v1
    acl:
      - actions: [publish]
`)
	doc, err := Load(data)
	require.NoError(t, err)
	plan, err := doc.Render(SeedContext{
		TenantUUID:    "6b5d0240-9920-46da-b707-88200e0f51ea",
		PluginID:      "com.powerx.plugins.base",
		PluginVersion: "0.1.1",
	})
	require.NoError(t, err)
	require.Len(t, plan.Topics, 1)
	require.Equal(t, "powerx.runtime.scheduler.triggered", plan.Topics[0].Topic.Namespace)
	require.Equal(t, "v1", plan.Topics[0].Topic.Name)
	require.Equal(t, "6b5d0240-9920-46da-b707-88200e0f51ea.powerx.runtime.scheduler.triggered.v1", plan.Topics[0].FullTopic)
}

func TestRenderPreservesRuntimeTopicTemplateTokens(t *testing.T) {
	data := []byte(`
version: v1
topics:
  - topic: ai_craft.shopify.product.sync.progress.member.tenant_{{tenant_uuid}}.member_{{member_uuid}}
    acl:
      - actions: [publish, subscribe]
`)
	doc, err := Load(data)
	require.NoError(t, err)
	plan, err := doc.Render(SeedContext{
		TenantUUID:    "6b5d0240-9920-46da-b707-88200e0f51ea",
		PluginID:      "com.powerx.plugins.ai-craft",
		PluginVersion: "0.1.0",
	})
	require.NoError(t, err)
	require.Len(t, plan.Topics, 1)
	require.Equal(t, "ai_craft.shopify.product.sync.progress.member.tenant_6b5d0240-9920-46da-b707-88200e0f51ea", plan.Topics[0].Topic.Namespace)
	require.Equal(t, "member_{{member_uuid}}", plan.Topics[0].Topic.Name)
	require.Equal(t, "6b5d0240-9920-46da-b707-88200e0f51ea.ai_craft.shopify.product.sync.progress.member.tenant_6b5d0240-9920-46da-b707-88200e0f51ea.member_{{member_uuid}}", plan.Topics[0].FullTopic)
}

func TestLoadRejectsUnsupportedRuntimeTopicTemplateToken(t *testing.T) {
	data := []byte(`
version: v1
topics:
  - topic: ai_craft.bad.member_{{account_uuid}}
`)
	_, err := Load(data)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported topic template token")
}

func TestLoadRejectsDottedExplicitName(t *testing.T) {
	data := []byte(`
version: v1
topics:
  - key: powerx.runtime.scheduler.triggered.v1
    namespace: powerx.runtime.scheduler
    name: triggered.v1
`)
	_, err := Load(data)
	require.Error(t, err)
	require.Contains(t, err.Error(), "topic[0] name must match")
}
