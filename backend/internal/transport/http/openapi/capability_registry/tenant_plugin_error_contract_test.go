package capability_registry

import (
	"encoding/json"
	"errors"
	"testing"

	pm "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
)

func TestPluginServiceBusinessErrorThroughAuthenticatedHTTPEntry(t *testing.T) {
	const trace = "12345678-1234-1234-1234-123456789abc"
	for _, test := range []struct {
		name, body, code string
		status           int
	}{
		{"idempotency conflict", `{"error_code":"MESSAGING_IDEMPOTENCY_CONFLICT","trace_id":"` + trace + `","message":"private-upstream-message","details":{"retryable":false,"operation_uuid":"` + trace + `","body":"private-message-content"}}`, "MESSAGING_IDEMPOTENCY_CONFLICT", 409},
		{"SCRM nested envelope", `{"success":false,"error":{"code":"MESSAGING_IDEMPOTENCY_CONFLICT","message":"private-localized-message","details":{"next_action_code":"inspect_existing_request","raw":"private-payload"}},"request_id":"` + trace + `"}`, "MESSAGING_IDEMPOTENCY_CONFLICT", 409},
		{"temporary failure", `{"error_code":"MESSAGING_TEMPORARILY_UNAVAILABLE","details":{"retryable":true,"retry_after_ms":1000}}`, "MESSAGING_TEMPORARILY_UNAVAILABLE", 503},
		{"invalid body", `private-upstream-response`, "CAPABILITY_PLUGIN_REJECTED", 422},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newInvocationHTTPFixture(t, true)
			f.responseStatus = test.status
			f.responseBody = test.body
			w := f.request(t, "/api/v1/tenant/invocations", map[string]interface{}{"capability_id": f.capID, "preferred_protocol": "rest", "payload": map[string]interface{}{"body": map[string]interface{}{"text": "fixture"}}})
			require.Equal(t, test.status, w.Code, w.Body.String())
			var envelope map[string]interface{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
			require.Equal(t, test.code, envelope["error_code"])
			require.Equal(t, test.code, envelope["reason_code"])
			details := envelope["details"].(map[string]interface{})
			require.EqualValues(t, test.status, details["upstream_status"])
			require.NotEmpty(t, details["trace_id"])
			require.NotContains(t, w.Body.String(), "private-")
			require.EqualValues(t, 1, f.calls.Load())
			require.Equal(t, 1, f.issuer.calls)
			if test.name == "SCRM nested envelope" {
				require.Equal(t, trace, details["upstream_request_id"])
				require.Equal(t, "inspect_existing_request", details["next_action_code"])
			}
			if test.name == "idempotency conflict" {
				require.Equal(t, trace, details["upstream_trace_id"])
				require.Equal(t, trace, details["operation_uuid"])
			}
		})
	}
}

func TestPluginRuntimeFailureThroughAuthenticatedHTTPEntry(t *testing.T) {
	for _, test := range []struct {
		name   string
		alter  func(*invocationHTTPFixture)
		status int
		code   string
	}{
		{"version mismatch", func(f *invocationHTTPFixture) { f.runtime.Version = "1.2.2" }, 409, "CAPABILITY_PLUGIN_RUNTIME_VERSION_MISMATCH"},
		{"disabled", func(f *invocationHTTPFixture) { f.runtime.State = pm.StateDisabled }, 503, "CAPABILITY_PLUGIN_RUNTIME_UNAVAILABLE"},
		{"lookup unavailable", func(f *invocationHTTPFixture) { f.lookupErr = errors.New("private-lookup") }, 503, "CAPABILITY_PLUGIN_RUNTIME_UNAVAILABLE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newInvocationHTTPFixture(t, true)
			test.alter(f)
			w := f.request(t, "/api/v1/tenant/invocations", map[string]interface{}{"capability_id": f.capID, "preferred_protocol": "rest", "payload": map[string]interface{}{"body": map[string]interface{}{}}})
			require.Equal(t, test.status, w.Code, w.Body.String())
			var envelope map[string]interface{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
			require.Equal(t, test.code, envelope["error_code"])
			require.Equal(t, test.code, envelope["reason_code"])
			require.NotContains(t, w.Body.String(), "private-")
			require.Zero(t, f.calls.Load())
			require.Zero(t, f.issuer.calls)
			details := envelope["details"].(map[string]interface{})
			require.NotEmpty(t, details["trace_id"])
			if test.status == 409 {
				require.Equal(t, "1.2.3", details["expected_provider_version"])
				require.Equal(t, "1.2.2", details["actual_provider_version"])
			}
		})
	}
}
