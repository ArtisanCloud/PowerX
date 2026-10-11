package capability_registry

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestIdentityTicketTraceRedactionDoesNotMutateExecution(t *testing.T) {
	original := map[string]interface{}{"body": map[string]interface{}{"operation": "resolve_request_identity", "request": map[string]interface{}{"context_ticket": "opaque-secret", "schema": "v1"}}}
	redacted := redactIdentityTicketPayload("com.corex.iam.identity_context.service_read", original)
	get := func(x map[string]interface{}) any {
		return x["body"].(map[string]interface{})["request"].(map[string]interface{})["context_ticket"]
	}
	require.Equal(t, "[REDACTED]", get(redacted))
	require.Equal(t, "opaque-secret", get(original))
}

func TestWSHandleResponseStructIsRedactedWithoutMutation(t *testing.T) {
	result := struct {
		Handle string `json:"ws_revalidation_handle"`
		Tenant string `json:"tenant_uuid"`
	}{Handle: "server-only-proof", Tenant: "tenant"}
	original := map[string]interface{}{"payload": map[string]interface{}{"result": result}}
	copy := redactIdentityTicketPayload("com.corex.iam.identity_context.service_read", original)
	redacted := copy["payload"].(map[string]interface{})["result"].(map[string]interface{})
	require.Equal(t, "[REDACTED]", redacted["ws_revalidation_handle"])
	require.Equal(t, "server-only-proof", result.Handle)
	require.Equal(t, "tenant", redacted["tenant_uuid"])
}
