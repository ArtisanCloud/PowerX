package capability_registry

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	"github.com/stretchr/testify/require"
)

func TestRESTRegistrationAdapterIDUsesStableMethodAndPathIdentity(t *testing.T) {
	const id = "com.example.routes.read"
	const plugin = "com.example.routes"
	first := models.ProtocolBinding{Channel: "rest", Method: "GET", Endpoint: "/api/v1/items"}
	second := first
	second.Endpoint = "/api/v1/items/{item_id}"
	a := registrationAdapterFromProtocol(id, plugin, first)
	b := registrationAdapterFromProtocol(id, plugin, second)
	require.NotEqual(t, a.AdapterID, b.AdapterID)
	require.Equal(t, a, registrationAdapterFromProtocol(id, plugin, first))
	require.Equal(t, b, registrationAdapterFromProtocol(id, plugin, second))
	// 规范化后的同一路由和大小写／空白规范化仍标识为同一绑定，不能逃过重复校验。
	equivalent := first
	equivalent.Channel = " REST "
	equivalent.Method = " get "
	equivalent.Endpoint = a.Endpoint
	require.Equal(t, a.AdapterID, registrationAdapterFromProtocol(id, plugin, equivalent).AdapterID)
	changedMethod := first
	changedMethod.Method = "POST"
	require.NotEqual(t, a.AdapterID, registrationAdapterFromProtocol(id, plugin, changedMethod).AdapterID)
	require.Equal(t, "GET", a.Labels["method"])
	require.Equal(t, "/_p/"+plugin+first.Endpoint, a.Endpoint)
	require.LessOrEqual(t, len(registrationAdapterFromProtocol(strings.Repeat("x", 256), plugin, first).AdapterID), 512)
	for _, channel := range []string{"http", "HTTP"} {
		p := first
		p.Channel = channel
		q := p
		q.Endpoint = second.Endpoint
		require.NotEqual(t, registrationAdapterFromProtocol(id, plugin, p).AdapterID, registrationAdapterFromProtocol(id, plugin, q).AdapterID)
	}
	// 调换输入顺序不改变任何 ID。
	forward := []string{a.AdapterID, b.AdapterID}
	reverse := []string{registrationAdapterFromProtocol(id, plugin, second).AdapterID, registrationAdapterFromProtocol(id, plugin, first).AdapterID}
	require.Equal(t, forward[0], reverse[1])
	require.Equal(t, forward[1], reverse[0])
}

func TestRegistrationAdapterIDKeepsExplicitToolAndRPCIdentities(t *testing.T) {
	for _, test := range []struct{ channel, tool, rpc, want string }{
		{"rest", "declared-tool", "Example.Read", "declared-tool"},
		{"rest", "", "Example.Read", "Example.Read"},
		{"grpc", "", "Example.Read", "Example.Read"},
		{"mcp", "declared-tool", "", "declared-tool"},
	} {
		p := models.ProtocolBinding{Channel: test.channel, Method: "POST", Endpoint: "/first", ToolRef: test.tool, RPC: test.rpc}
		first := registrationAdapterFromProtocol("example.cap", "example.plugin", p)
		p.Endpoint = "/second"
		require.Equal(t, "example.cap."+test.want, first.AdapterID)
		require.Equal(t, first.AdapterID, registrationAdapterFromProtocol("example.cap", "example.plugin", p).AdapterID)
	}
}

func TestDuplicateRESTBindingStillFailsRegistrationInsteadOfBeingRenamed(t *testing.T) {
	db := newMemoryDB(t)
	root := buildSamplePlugin(t, true)
	file := filepath.Join(root, "capabilities/catalog.json")
	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	var catalog map[string]any
	require.NoError(t, json.Unmarshal(raw, &catalog))
	capability := catalog["capabilities"].([]any)[0].(map[string]any)
	bindings := capability["protocols"].([]any)
	require.NotEmpty(t, bindings)
	capability["protocols"] = append(bindings, bindings[0])
	raw, err = json.Marshal(catalog)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, raw, 0600))
	err = NewSyncWorker(SyncWorkerConfig{DB: db, TenantUUID: "11111111-1111-1111-1111-111111111111"}).ProcessArtifact(context.Background(), root)
	require.ErrorContains(t, err, "duplicate adapter id")
	var count int64
	require.NoError(t, db.Model(&models.CapabilityRegistration{}).Count(&count).Error)
	require.Zero(t, count)
	var job models.CapabilitySyncJob
	require.NoError(t, db.First(&job).Error)
	require.Equal(t, "failed", job.Status)
}

func TestProtocolMapOrderDoesNotChangeRegistrationIdentityOrHash(t *testing.T) {
	protocols := map[string]any{
		"rest":       []any{map[string]any{"method": "GET", "path": "/api/v1/items"}, map[string]any{"method": "GET", "path": "/api/v1/items/{id}"}},
		"grpc":       map[string]any{"rpc": "Example.Items/Read", "endpoint": "grpc://items"},
		"agent_tool": map[string]any{"tool_ref": "items.read", "endpoint": "agent://items/read"},
	}
	first, err := buildProtocolBindings(protocols, capabilityManifestProvide{})
	require.NoError(t, err)
	expected := hashJSON(first)
	for repeat := 0; repeat < 20; repeat++ {
		rebuilt, err := buildProtocolBindings(protocols, capabilityManifestProvide{})
		require.NoError(t, err)
		require.Equal(t, first, rebuilt)
		require.Equal(t, expected, hashJSON(rebuilt))
	}
	require.Equal(t, "agent_tool", first[0].Channel)
	require.Equal(t, "grpc", first[1].Channel)
	require.Equal(t, "rest", first[2].Channel)
	require.Equal(t, "/api/v1/items", first[2].Endpoint)
	require.Equal(t, "/api/v1/items/{id}", first[3].Endpoint)
}
