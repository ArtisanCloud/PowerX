package capability_registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	identity "github.com/ArtisanCloud/PowerX/internal/service/plugin_identity"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
)

func TestIAMTypedInvocationPreservesBusinessError(t *testing.T) {
	tests := []struct {
		status int
		reason string
	}{
		{400, "IAM_CONTEXT_INVALID_ARGUMENT"},
		{401, "IAM_CONTEXT_WS_HANDLE_EXPIRED"},
		{401, "IAM_CONTEXT_USER_SESSION_INVALID"},
		{401, "IAM_CONTEXT_PROOF_EXPIRED"},
		{403, "IAM_CONTEXT_REQUEST_FORBIDDEN"},
		{403, "IAM_CONTEXT_PLUGIN_FORBIDDEN"},
		{409, "IAM_CONTEXT_POLICY_CHANGED"},
		{503, "IAM_CONTEXT_UNAVAILABLE"},
	}
	for _, tt := range tests {
		for _, wrapped := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/wrapped=%t", tt.reason, wrapped), func(t *testing.T) {
				var err error = dto.NewErrorWithCode(tt.status, tt.reason, tt.reason, nil)
				if wrapped {
					err = fmt.Errorf("typed invocation: %w", err)
				}
				invoker := &externalIdentityContractInvoker{err: err}
				response := invokeExternalIdentityContract(t, externalIdentityContractEngine(invoker, true), iamWSInvocationRequest())
				require.Equal(t, tt.status, response.Code)
				var envelope map[string]interface{}
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
				require.EqualValues(t, tt.status, envelope["code"])
				require.Equal(t, tt.reason, envelope["error_code"])
				require.Equal(t, tt.reason, envelope["reason_code"])
				details := envelope["details"].(map[string]interface{})
				require.Equal(t, tt.reason, details["reason_code"])
				require.Equal(t, "trace-iam-typed-error", details["trace_id"])
				require.NotContains(t, response.Body.String(), "ws_revalidation_handle")
			})
		}
	}
}

func TestIAMBaselineTypedInvocationPreservesStructuredErrors(t *testing.T) {
	for _, status := range []int{400, 401, 403, 409, 503} {
		code := "IAM_BASELINE_UNAVAILABLE"
		invoker := &externalIdentityContractInvoker{err: dto.NewErrorWithCode(status, code, code, nil)}
		request := map[string]interface{}{"capability_id": "com.corex.iam.authorization_baseline.service_read", "preferred_protocol": "core_internal", "payload": map[string]interface{}{"body": map[string]interface{}{"operation": "read_member_baselines"}}}
		response := invokeExternalIdentityContract(t, externalIdentityContractEngine(invoker, true), request)
		require.Equal(t, status, response.Code)
		var envelope map[string]interface{}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
		require.Equal(t, code, envelope["error_code"])
		require.Equal(t, code, envelope["reason_code"])
	}
}

func TestIAMTypedInvocationUnclassifiedFailureRemainsBadGateway(t *testing.T) {
	for _, err := range []error{
		errors.New("IAM_CONTEXT_WS_HANDLE_EXPIRED"), // 字符串不能冒充业务错误类型。
		&capservice.RESTUpstreamError{StatusCode: http.StatusUnauthorized, RemoteCode: "IAM_CONTEXT_WS_HANDLE_EXPIRED"},
	} {
		invoker := &externalIdentityContractInvoker{err: err}
		response := invokeExternalIdentityContract(t, externalIdentityContractEngine(invoker, true), iamWSInvocationRequest())
		require.Equal(t, http.StatusBadGateway, response.Code)
	}
}

func iamWSInvocationRequest() map[string]interface{} {
	return map[string]interface{}{
		"capability_id": identity.CapabilityID, "preferred_protocol": "core_internal", "trace_id": "trace-iam-typed-error",
		"payload": map[string]interface{}{"body": map[string]interface{}{
			"operation": "revalidate_ws_identity",
			"request":   map[string]interface{}{"schema": identity.WSQuerySchema, "ws_revalidation_handle": "server-only-test-handle"},
		}},
	}
}
