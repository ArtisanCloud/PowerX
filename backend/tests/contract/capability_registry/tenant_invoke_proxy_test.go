package capabilityregistrycontract

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	capabilityregistryhttp "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/capability_registry"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const tenantInvokeUUID = "aeffc79f-e72a-4fd9-b908-5c150bce3741"

func TestTenantInvokeProxyReturnsPayloadForREST(t *testing.T) {
	gin.SetMode(gin.TestMode)

	resultPayload := map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{
				"uuid": "asset-1",
				"name": "demo-image.png",
			},
		},
		"pagination": map[string]interface{}{
			"page":      1.0,
			"page_size": 20.0,
			"total":     1.0,
		},
	}
	engine := setupTenantInvokeEngine(t, capservice.InvocationResult{
		TraceID:      "trace-http",
		Status:       "completed",
		ProtocolUsed: "http",
		FallbackUsed: false,
		Result:       resultPayload,
	})

	data := invokeTenantGateway(t, engine, map[string]interface{}{
		"capability_id":      "com.corex.media.assets.read",
		"preferred_protocol": "rest",
		"payload": map[string]interface{}{
			"method":   "GET",
			"endpoint": "/api/v1/media/assets",
		},
	})

	require.Equal(t, "trace-http", data["trace_id"])
	require.Equal(t, "completed", data["status"])
	require.Equal(t, "http", data["protocol_used"])
	require.Equal(t, false, data["fallback_used"])

	payload := requirePayload(t, data)
	require.Equal(t, resultPayload, payload, "HTTP payload should be proxied verbatim")

	legacy, ok := data["result"].(map[string]interface{})
	require.True(t, ok, "legacy result field missing")
	require.Equal(t, resultPayload, legacy, "legacy result should mirror payload for backward compatibility")
}

func TestTenantInvokeProxyMarksFallbackMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)

	resultPayload := map[string]interface{}{
		"grpc": map[string]interface{}{
			"endpoint": "powerx.media.v1.MediaAssetAdminService",
			"rpc":      "ListMediaAssets",
		},
	}
	engine := setupTenantInvokeEngine(t, capservice.InvocationResult{
		TraceID:      "trace-grpc",
		Status:       "completed",
		ProtocolUsed: "grpc",
		FallbackUsed: true,
		Result:       resultPayload,
	})

	data := invokeTenantGateway(t, engine, map[string]interface{}{
		"capability_id":      "com.corex.media.assets.read",
		"preferred_protocol": "grpc",
		"payload": map[string]interface{}{
			"endpoint": "powerx.media.v1.MediaAssetAdminService",
			"rpc":      "ListMediaAssets",
			"body": map[string]interface{}{
				"page":      1,
				"page_size": 5,
			},
		},
	})

	require.Equal(t, "trace-grpc", data["trace_id"])
	require.Equal(t, "grpc", data["protocol_used"])
	require.Equal(t, true, data["fallback_used"], "fallback flag should surface to clients")

	payload := requirePayload(t, data)
	require.Equal(t, resultPayload, payload, "gRPC payload should be proxied verbatim")
}

func setupTenantInvokeEngine(t *testing.T, result capservice.InvocationResult) *gin.Engine {
	t.Helper()

	fakeInvoker := &fakeCapabilityInvoker{
		result: result,
	}
	selector := capservice.NewSelector(capservice.SelectorOptions{
		Invoker: fakeInvoker,
	})
	deps := &shared.Deps{
		CapabilityCatalogSvc: &capservice.RegistryService{},
		CapabilitySelector:   selector,
	}

	engine := gin.New()
	capabilityregistryhttp.RegisterTenantRoutes(engine.Group("/api/v1"), deps)
	return engine
}

func invokeTenantGateway(t *testing.T, engine *gin.Engine, body map[string]interface{}) map[string]interface{} {
	t.Helper()

	data, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenant/invocations", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("tenant-uuid", tenantInvokeUUID)

	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code, "proxy should return 200 on successful invocation")

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &envelope))

	codeValue, ok := envelope["code"].(float64)
	require.True(t, ok, "response missing numeric code field")
	require.Equal(t, float64(http.StatusOK), codeValue)

	dataField, ok := envelope["data"].(map[string]interface{})
	require.True(t, ok, "response missing data envelope")
	return dataField
}

func requirePayload(t *testing.T, data map[string]interface{}) map[string]interface{} {
	t.Helper()
	payload, ok := data["payload"].(map[string]interface{})
	require.True(t, ok, "response missing payload field")
	return payload
}

type fakeCapabilityInvoker struct {
	result capservice.InvocationResult
}

func (f *fakeCapabilityInvoker) Invoke(ctx context.Context, in capservice.InvocationInput) (capservice.InvocationResult, error) {
	return f.result, nil
}
