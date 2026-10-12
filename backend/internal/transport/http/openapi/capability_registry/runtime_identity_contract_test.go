package capability_registry

import (
	"encoding/json"
	"github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	identity "github.com/ArtisanCloud/PowerX/internal/service/runtime_identity"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRuntimeIdentityGatewayPreservesErrorsAndCachePolicy(t *testing.T) {
	for _, status := range []int{400, 403, 404, 503} {
		code := "RUNTIME_IDENTITY_UNAVAILABLE"
		invoker := &externalIdentityContractInvoker{err: identity.Error(status, code)}
		engine := externalIdentityContractEngine(invoker, true)
		response := invokeExternalIdentityContract(t, engine, map[string]interface{}{"capability_id": identity.CapabilityID, "preferred_protocol": "core_internal", "payload": map[string]interface{}{"body": map[string]interface{}{"operation": "get", "plugin_id": "plugin.test"}}})
		require.Equal(t, status, response.Code)
		require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
		var envelope map[string]interface{}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
		require.Equal(t, code, envelope["error_code"])
		require.Equal(t, code, envelope["reason_code"])
	}
	invoker := &externalIdentityContractInvoker{result: capability_registry.InvocationResult{Status: "completed", TraceID: "trace-identity", ProtocolUsed: "core_internal", Result: map[string]interface{}{"item": map[string]interface{}{"plugin_version_source": "registry"}}}}
	response := invokeExternalIdentityContract(t, externalIdentityContractEngine(invoker, true), map[string]interface{}{"capability_id": identity.CapabilityID})
	require.Equal(t, 200, response.Code)
	require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
}
