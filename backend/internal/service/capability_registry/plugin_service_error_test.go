package capability_registry

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	pm "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
)

func TestPluginServiceErrorSanitizesUntrustedDiagnostics(t *testing.T) {
	const trace = "12345678-1234-1234-1234-123456789abc"
	cases := []struct{ name, raw, code string }{
		{"business", `{"error_code":"MESSAGING_IDEMPOTENCY_CONFLICT","trace_id":"` + trace + `","message":"secret-message","error":"secret-error","details":{"retryable":true,"retry_after_ms":2500,"operation_uuid":"` + trace + `","recovery":{"retry_after":5,"operation_id":"` + trace + `","url":"https://secret.invalid"},"raw":"secret-details","authorization":"secret-bearer"}}`, "MESSAGING_IDEMPOTENCY_CONFLICT"},
		{"malformed", `secret-plaintext`, "CAPABILITY_PLUGIN_REJECTED"},
		{"trailing", `{"error_code":"MESSAGING_CONFLICT"} secret`, "CAPABILITY_PLUGIN_REJECTED"},
		{"ambiguous codes", `{"error_code":"MESSAGING_A","success":false,"error":{"code":"MESSAGING_B"}}`, "CAPABILITY_PLUGIN_REJECTED"},
		{"false success envelope", `{"success":true,"error":{"code":"MESSAGING_A"}}`, "CAPABILITY_PLUGIN_REJECTED"},
		{"invalid code", `{"error_code":"MESSAGING secret"}`, "CAPABILITY_PLUGIN_REJECTED"},
		{"forged Core code", `{"error_code":"IAM_FORBIDDEN"}`, "CAPABILITY_PLUGIN_REJECTED"},
		{"invalid recovery", `{"error_code":"MESSAGING_CONFLICT","trace_id":"secret-trace","details":{"retryable":"secret","retry_after_ms":-1,"retry_after":1e9,"operation_id":"secret-operation","current_revision":1.2,"recovery":{"operation_uuid":"secret"}}}`, "MESSAGING_CONFLICT"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			e := newPluginServiceUpstreamError(409, "POST", "/api/v1/service/reply", []byte(test.raw))
			require.Equal(t, test.code, e.BusinessCode)
			require.True(t, e.PreserveStatus())
			raw, err := json.Marshal(InvocationFailureDetails(e))
			require.NoError(t, err)
			require.NotContains(t, string(raw), "secret")
			require.NotContains(t, e.Error(), "secret")
			if test.name == "business" {
				require.Equal(t, trace, e.RemoteTraceID)
				require.EqualValues(t, 2500, e.Details["retry_after_ms"])
				require.Equal(t, trace, e.Details["recovery"].(map[string]interface{})["operation_id"])
			}
		})
	}
}

func TestPluginServiceRuntimeFailuresAreStructured(t *testing.T) {
	require.NoError(t, ValidatePluginServiceRuntime(pm.Plugin{Version: "1.2.3", State: pm.StateEnabled}, "1.2.3", nil))
	for _, test := range []struct {
		plugin pm.Plugin
		lookup error
		code   string
		status int
	}{
		{pm.Plugin{Version: "old", State: pm.StateEnabled}, nil, "CAPABILITY_PLUGIN_RUNTIME_VERSION_MISMATCH", 409},
		{pm.Plugin{Version: "1.2.3", State: pm.StateDisabled}, nil, "CAPABILITY_PLUGIN_RUNTIME_UNAVAILABLE", 503},
		{pm.Plugin{}, errors.New("private runtime lookup failure"), "CAPABILITY_PLUGIN_RUNTIME_UNAVAILABLE", 503},
	} {
		err := ValidatePluginServiceRuntime(test.plugin, "1.2.3", test.lookup)
		require.Equal(t, test.code, dto.CodeOf(err))
		require.Equal(t, test.status, dto.StatusCode(err))
		require.NotContains(t, err.Error(), "private")
	}
}
