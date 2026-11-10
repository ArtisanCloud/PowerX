package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	am "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func connectionTestBody(provider string) map[string]any {
	return map[string]any{"contract_version": float64(1), "operation": "test_connection", "modality": "llm", "provider": provider}
}

func connectionTestInput(body map[string]any) cap.CoreCapabilityInvokeInput {
	return cap.CoreCapabilityInvokeInput{CapabilityID: ConnectionTestCapability, TenantUUID: mediaTestTenant, Method: "INVOKE", Endpoint: ConnectionTestEndpoint, Body: body, Payload: map[string]any{"body": body}}
}

func TestConnectionBindingUsesSavedCredentialsAndNeverGeneratesOrSaves(t *testing.T) {
	f := newMediaFixture(t)
	const secret = "connection-test-secret"
	var requests atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/v1/models", r.URL.Path)
		require.Equal(t, "tenant-organization", r.Header.Get("OpenAI-Organization"))
		if r.Header.Get("Authorization") != "Bearer "+secret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(provider.Close)
	require.NoError(t, f.s.ai.settings.SaveCredentialOnly(f.ctx, "dev", ptr(mediaTestTenant), &am.AIProviderCredential{Name: "dev-openai", Provider: "openai", AuthScheme: "bearer", Data: datatypes.JSONMap{"api_key": secret, "base_url": provider.URL + "/v1", "organization": "tenant-organization"}}))
	var before []am.AIProviderCredential
	require.NoError(t, f.db.Find(&before).Error)
	result, err := (&MediaInvoker{Service: f.s}).InvokeCoreCapability(f.ctx, connectionTestInput(connectionTestBody("openai")))
	require.NoError(t, err)
	out := result["result"].(ConnectionTestResult)
	require.True(t, out.OK)
	require.True(t, out.Verification.APIKeyVerified)
	require.False(t, out.Verification.GenerationSubmitted)
	require.False(t, out.Verification.ModelAccessVerified)
	require.EqualValues(t, 2, requests.Load())
	var after []am.AIProviderCredential
	require.NoError(t, f.db.Find(&after).Error)
	require.Equal(t, before, after, "probe must not modify credential data or timestamps")
	raw, err := json.Marshal(out)
	require.NoError(t, err)
	require.NotContains(t, string(raw), secret)
	require.NotContains(t, string(raw), provider.URL)
	doc, err := openapi3.NewLoader().LoadFromFile("../../../../docs/contracts/ai-connection-test.v1.openapi.yaml")
	require.NoError(t, err)
	require.NoError(t, doc.Components.Schemas["ConnectionTestOperation"].Value.VisitJSON(connectionTestBody("openai")))
	var object any
	require.NoError(t, json.Unmarshal(raw, &object))
	require.NoError(t, doc.Components.Schemas["ConnectionTestResult"].Value.VisitJSON(object))
	require.Zero(t, f.generations.Load())
}

func TestConnectionBindingRejectsOverridesBeforeAnyProbe(t *testing.T) {
	f := newMediaFixture(t)
	i := &MediaInvoker{Service: f.s}
	for _, name := range []string{"env", "tenant_uuid", "base_url", "baseURL", "endpoint", "method", "headers", "api_key", "apiKey", "prompt", "inputs", "params", "context", "save"} {
		body := connectionTestBody("openai")
		body[name] = "caller-selected"
		_, err := i.InvokeCoreCapability(f.ctx, connectionTestInput(body))
		requireMediaCode(t, err, "AI_OPERATION_INVALID")
	}
	for _, key := range []string{"endpoint", "method", "headers", "context"} {
		input := connectionTestInput(connectionTestBody("openai"))
		input.Payload[key] = ""
		_, err := i.InvokeCoreCapability(f.ctx, input)
		requireMediaCode(t, err, "AI_TYPED_OPERATION_REQUIRED")
	}
	for _, change := range []map[string]any{{"contract_version": 0}, {"operation": "invoke"}, {"modality": "unknown"}, {"provider": ""}, {"provider": " openai"}} {
		body := connectionTestBody("openai")
		for key, value := range change {
			body[key] = value
		}
		_, err := i.InvokeCoreCapability(f.ctx, connectionTestInput(body))
		requireMediaCode(t, err, "AI_OPERATION_INVALID")
	}
	cross := reqctx.WithTenantUUID(f.ctx, "22222222-2222-4222-8222-222222222222")
	_, err := i.InvokeCoreCapability(cross, connectionTestInput(connectionTestBody("ollama")))
	requireMediaCode(t, err, "AI_FORBIDDEN")
	require.NoError(t, f.db.Where("tenant_uuid = ? AND key = ?", mediaTestTenant, "ai.current_env").Delete(&setting.TenantSetting{}).Error)
	_, err = i.InvokeCoreCapability(f.ctx, connectionTestInput(connectionTestBody("ollama")))
	requireMediaCode(t, err, "AI_ENV_NOT_CONFIGURED")
	require.Zero(t, f.generations.Load())
}

func TestConnectionBindingRequiresExactSavedTenantEnvironment(t *testing.T) {
	f := newMediaFixture(t)
	var requests atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/tags", r.URL.Path)
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	t.Cleanup(provider.Close)
	i := &MediaInvoker{Service: f.s}
	save := func(env string, tenant *string) {
		require.NoError(t, f.s.ai.settings.SaveCredentialOnly(context.Background(), env, tenant, &am.AIProviderCredential{Name: env + "-ollama", Provider: "ollama", Data: datatypes.JSONMap{"base_url": provider.URL}}))
	}
	save("staging", ptr(mediaTestTenant))
	save("dev", ptr("22222222-2222-4222-8222-222222222222"))
	save("dev", nil)
	_, err := i.InvokeCoreCapability(f.ctx, connectionTestInput(connectionTestBody("ollama")))
	requireMediaCode(t, err, "AI_SETTINGS_PROVIDER_NOT_CONFIGURED")
	require.Zero(t, requests.Load())
	save("dev", ptr(mediaTestTenant))
	out, err := i.InvokeCoreCapability(f.ctx, connectionTestInput(connectionTestBody("ollama")))
	require.NoError(t, err)
	verification := out["result"].(ConnectionTestResult).Verification
	require.True(t, verification.EndpointReachable)
	require.False(t, verification.APIKeyVerified)
	require.Equal(t, "not_required", verification.Authentication)
	require.EqualValues(t, 1, requests.Load())
}
