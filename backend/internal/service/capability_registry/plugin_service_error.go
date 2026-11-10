package capability_registry

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	pm "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/google/uuid"
)

// ValidatePluginServiceRuntime is shared by runtime resolution and its HTTP
// contract tests. Runtime failures are Core errors, never upstream free text.
func ValidatePluginServiceRuntime(plugin pm.Plugin, expectedVersion string, lookupErr error) error {
	if lookupErr != nil || plugin.State != pm.StateEnabled {
		return pluginServiceError(http.StatusServiceUnavailable, "CAPABILITY_PLUGIN_RUNTIME_UNAVAILABLE")
	}
	if plugin.Version != expectedVersion {
		return dto.WithDetails(pluginServiceError(http.StatusConflict, "CAPABILITY_PLUGIN_RUNTIME_VERSION_MISMATCH"), map[string]interface{}{
			"expected_provider_version": expectedVersion,
			"actual_provider_version":   plugin.Version,
		})
	}
	return nil
}

// newPluginServiceUpstreamError deliberately never forwards messages, error
// strings, arbitrary details or raw response bodies from another plugin.
func newPluginServiceUpstreamError(status int, method, endpoint string, raw []byte) *RESTUpstreamError {
	e := &RESTUpstreamError{StatusCode: status, Method: method, Endpoint: endpoint,
		RemoteCode: "CAPABILITY_PLUGIN_REJECTED", BusinessCode: "CAPABILITY_PLUGIN_REJECTED",
		Cause: "Plugin rejected the capability invocation", SafePlugin: true}
	var body map[string]interface{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&body) != nil || body == nil {
		return e
	}
	// Reject trailing data instead of trusting a valid JSON prefix.
	var trailing interface{}
	if decoder.Decode(&trailing) != io.EOF {
		return e
	}
	code, _ := body["error_code"].(string)
	details, _ := body["details"].(map[string]interface{})
	if nested, ok := body["error"].(map[string]interface{}); ok {
		// SCRM's formal APIResponse uses success=false + error.code/details.
		if success, ok := body["success"].(bool); !ok || success {
			return e
		}
		nestedCode, _ := nested["code"].(string)
		if flat, exists := body["error_code"]; exists && flat != nestedCode {
			return e
		}
		code = nestedCode
		details, _ = nested["details"].(map[string]interface{})
	}
	if !validRESTBusinessCode(code) {
		return e
	}
	// A plugin cannot impersonate Core authentication/runtime diagnostics.
	for _, prefix := range []string{"CAPABILITY_", "IAM_", "STS_", "RUNTIME_IDENTITY_"} {
		if strings.HasPrefix(code, prefix) {
			return e
		}
	}
	e.RemoteCode, e.BusinessCode = code, code
	e.Details = safePluginRecovery(details)
	if trace, ok := body["trace_id"].(string); ok && validServiceTraceID(trace) {
		e.RemoteTraceID = trace
	}
	if requestID, ok := body["request_id"].(string); ok && validServiceUUID(requestID) {
		e.Details["upstream_request_id"] = requestID
	}
	if recovery, ok := details["recovery"].(map[string]interface{}); ok {
		if safe := safePluginRecovery(recovery); len(safe) > 0 {
			e.Details["recovery"] = safe
		}
	}
	return e
}

func validServiceTraceID(value string) bool {
	if validServiceUUID(value) {
		return true
	}
	if len(value) != 32 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validServiceUUID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}

func safePluginRecovery(source map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{}
	if value, ok := source["next_action_code"].(string); ok && validRecoveryActionCode(value) {
		result["next_action_code"] = value
	}
	if value, ok := source["retryable"].(bool); ok {
		result["retryable"] = value
	}
	for key, limit := range map[string]int64{"retry_after_ms": 86400000, "retry_after": 86400, "current_revision": 1<<53 - 1, "expected_revision": 1<<53 - 1} {
		if value, ok := source[key].(json.Number); ok {
			if n, err := value.Int64(); err == nil && n >= 0 && n <= limit {
				result[key] = n
			}
		}
	}
	for _, key := range []string{"operation_id", "operation_uuid", "request_id"} {
		if value, ok := source[key].(string); ok {
			if validServiceUUID(value) {
				result[key] = value
			}
		}
	}
	return result
}

func validRecoveryActionCode(value string) bool {
	if len(value) < 3 || len(value) > 64 {
		return false
	}
	for i, ch := range value {
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || i > 0 && (ch == '_' || ch >= '0' && ch <= '9') {
			continue
		}
		return false
	}
	return true
}
