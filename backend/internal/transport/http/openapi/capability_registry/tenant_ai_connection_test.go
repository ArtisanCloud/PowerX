package capability_registry

import (
	"encoding/json"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
)

func TestConnectionProbeHTTPPreservesProviderErrorsAndNoGenerationFlag(t *testing.T) {
	for _, tc := range []struct {
		status int
		code   string
	}{
		{422, "AI_CONNECTION_AUTH_FAILED"},
		{501, "AI_CONNECTION_PROBE_UNSUPPORTED"},
		{501, "AI_CONNECTION_AUTH_UNVERIFIABLE"},
		{504, "AI_CONNECTION_TIMEOUT"},
		{409, "AI_SETTINGS_PROVIDER_NOT_CONFIGURED"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			invoker := &externalIdentityContractInvoker{err: &dto.AppError{HTTPCode: tc.status, Code: tc.code, Message: tc.code, Details: map[string]any{"generation_submitted": false, "upstream_status": 401}}}
			response := invokeExternalIdentityContract(t, externalIdentityContractEngine(invoker, true), map[string]any{
				"capability_id": "com.corex.ai.connection.test", "preferred_protocol": "core_internal", "trace_id": "probe-http-test",
				"payload": map[string]any{"body": map[string]any{"contract_version": 1, "operation": "test_connection", "provider": "openai", "modality": "llm"}},
			})
			require.Equal(t, tc.status, response.Code)
			var body map[string]any
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
			require.Equal(t, tc.code, body["error_code"])
			require.Equal(t, tc.code, body["reason_code"])
			details := body["details"].(map[string]any)
			require.Equal(t, false, details["generation_submitted"])
			require.EqualValues(t, 401, details["upstream_status"])
		})
	}
}
