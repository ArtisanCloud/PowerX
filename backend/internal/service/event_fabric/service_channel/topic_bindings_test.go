package servicechannel

import (
	"encoding/json"
	"testing"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/stretchr/testify/require"
)

func TestResolveTopicsUsesLiveServiceIdentityAndACL(t *testing.T) {
	for _, scenario := range []string{"publish", "consume", "owner", "tenant", "revoked_acl", "revoked_grant", "missing_binding", "raw_proxy", "unexpected_field"} {
		t.Run(scenario, func(t *testing.T) {
			f := setup(t)
			plugin, capability := producer, PublishCapability
			if scenario == "consume" {
				plugin, capability = consumer, ConsumeCapability
			}
			var credential setting.PluginInstanceConfig
			require.NoError(t, f.db.Where("tenant_uuid = ? AND plugin_id = ?", tenant, plugin).First(&credential).Error)
			var value map[string]any
			require.NoError(t, json.Unmarshal(credential.ValueJSON, &value))
			value["config"] = map[string]any{"runtime": map[string]any{"event_bridge": map[string]any{"tenant_uuid": tenant, "topic_bindings": map[string]string{"business.changed": f.topic.FullTopic}}}}
			raw, err := json.Marshal(value)
			require.NoError(t, err)
			require.NoError(t, f.db.Model(&credential).Update("value_json", raw).Error)
			body := map[string]any{"contract_version": Contract, "operation": "resolve_topics", "tenant_uuid": tenant, "plugin_id": plugin, "topic_keys": []string{"business.changed"}}
			if capability == ConsumeCapability {
				body["subscriber_id"] = plugin
			}
			want := ""
			switch scenario {
			case "owner":
				body["plugin_id"] = consumer
				want = "EVENT_FABRIC_OWNER_FORBIDDEN"
			case "tenant":
				body["tenant_uuid"] = "00000000-0000-0000-0000-000000000002"
				want = "EVENT_FABRIC_TENANT_FORBIDDEN"
			case "revoked_acl":
				require.NoError(t, f.db.Model(&m.AclBinding{}).Where("principal_id = ?", plugin).Update("status", 0).Error)
				want = "EVENT_FABRIC_TOPIC_FORBIDDEN"
			case "revoked_grant":
				value["allowed_capabilities"] = []string{}
				raw, err = json.Marshal(value)
				require.NoError(t, err)
				require.NoError(t, f.db.Model(&credential).Update("value_json", raw).Error)
				want = "EVENT_FABRIC_GRANT_REQUIRED"
			case "missing_binding":
				body["topic_keys"] = []string{"unknown"}
				want = "EVENT_FABRIC_TOPIC_BINDING_REQUIRED"
			case "raw_proxy":
				body["endpoint"] = "https://invalid.example"
				want = "EVENT_FABRIC_INVALID_ARGUMENT"
			case "unexpected_field":
				body["topics"] = []string{f.topic.FullTopic}
				want = "EVENT_FABRIC_INVALID_ARGUMENT"
			}
			result, err := (&Invoker{Service: f.svc}).InvokeCoreCapability(ctxFor(plugin), cap.CoreCapabilityInvokeInput{TenantUUID: tenant, CapabilityID: capability, Endpoint: Endpoint, Method: "INVOKE", Body: body})
			if want != "" {
				code(t, want, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tenant, result["tenant_uuid"])
			require.Equal(t, map[string]string{"business.changed": f.topic.FullTopic}, result["topic_bindings"])
		})
	}
}
