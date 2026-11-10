package capability_registry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/stretchr/testify/require"
)

func TestTenantInvocationPreservesExplicitRESTBusinessFailures(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 409, 422} {
		for _, wrapped := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/wrapped=%t", status, wrapped), func(t *testing.T) {
				const code = "COMMERCE_CONFIGURATION_IDEMPOTENCY_CONFLICT"
				var err error = &capservice.RESTUpstreamError{StatusCode: status, RemoteCode: code, BusinessCode: code, RemoteTraceID: "plugin-trace", Cause: "localized message"}
				if wrapped {
					err = fmt.Errorf("REST binding: %w", err)
				}
				invoker := &externalIdentityContractInvoker{err: err}
				response := invokeExternalIdentityContract(t, externalIdentityContractEngine(invoker, true), map[string]interface{}{"capability_id": "com.powerx.plugins.ecommerce.purchase.option.write", "preferred_protocol": "rest", "payload": map[string]interface{}{"idempotency_key": "fixture-key"}})
				require.Equal(t, status, response.Code)
				var result map[string]interface{}
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
				require.Equal(t, code, result["error_code"])
				require.Equal(t, code, result["reason_code"])
				require.Equal(t, "plugin-trace", result["details"].(map[string]interface{})["upstream_trace_id"])
			})
		}
	}
}

func TestTenantInvocationUnclassifiedAndServerRESTFailuresRemainBadGateway(t *testing.T) {
	for _, upstream := range []*capservice.RESTUpstreamError{
		{StatusCode: 409, RemoteCode: "COMMERCE_CONFIGURATION_IDEMPOTENCY_CONFLICT"},
		{StatusCode: 503, RemoteCode: "COMMERCE_CONFIGURATION_UNAVAILABLE", BusinessCode: "COMMERCE_CONFIGURATION_UNAVAILABLE"},
	} {
		invoker := &externalIdentityContractInvoker{err: upstream}
		response := invokeExternalIdentityContract(t, externalIdentityContractEngine(invoker, true), map[string]interface{}{"capability_id": "com.powerx.plugins.ecommerce.purchase.option.write", "payload": map[string]interface{}{}})
		require.Equal(t, http.StatusBadGateway, response.Code)
	}
}
