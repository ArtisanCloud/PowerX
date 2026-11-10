package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	am "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func settingsCatalogInput(id string, body map[string]any) cap.CoreCapabilityInvokeInput {
	return cap.CoreCapabilityInvokeInput{CapabilityID: id, TenantUUID: mediaTestTenant, Method: "INVOKE", Endpoint: MediaCatalogEndpoint, Body: body, Payload: map[string]any{"body": body}}
}

func invokeSettingsCatalog(t *testing.T, f *mediaFixture, id string, body map[string]any) map[string]any {
	t.Helper()
	result, err := (&MediaInvoker{Service: f.s}).InvokeCoreCapability(f.ctx, settingsCatalogInput(id, body))
	require.NoError(t, err)
	return result
}

func TestSettingsCatalogPreservesManagementProvidersAppsAndSchema(t *testing.T) {
	f := newMediaFixture(t)
	manifest, _ := catalog.GetGlobalAIRegister().Manifest("ollama")
	manifest.Name = "底座自有 Ollama 名称"
	coze, _ := catalog.GetGlobalAIRegister().Manifest("coze")
	coze.Apps = map[string]catalog.AppManifest{"assistant": {Name: "正式应用", Modalities: map[string]catalog.ModalityManifest{"llm": {Models: []catalog.ModelManifest{{ID: "custom:latest"}}}}}}
	body := map[string]any{"operation": settingsProvidersOperation, "modality": "llm"}
	out := invokeSettingsCatalog(t, f, settingsProvidersCapability, body)["result"].(SettingsProviderCatalog)
	require.Equal(t, "dev", out.Env)
	require.Equal(t, "ai_settings", out.CatalogKind)
	require.Equal(t, 1, out.ContractVersion)
	expected := f.s.ai.settings.Providers("llm")
	require.Len(t, out.Providers, len(expected))
	for index, provider := range expected {
		require.Equal(t, provider.ID, out.Providers[index].ID)
		require.Equal(t, provider.Name, out.Providers[index].Name)
		require.NotNil(t, out.Providers[index].Apps)
		require.Len(t, out.Providers[index].Apps, len(provider.Apps))
		for appIndex, app := range provider.Apps {
			require.Equal(t, SettingsCatalogApp{ID: app.ID, Name: app.Name}, out.Providers[index].Apps[appIndex])
		}
	}
	modelsBody := map[string]any{"operation": settingsModelsOperation, "modality": "llm", "provider": "coze", "app": "assistant"}
	models := invokeSettingsCatalog(t, f, MediaCatalogCapability, modelsBody)["result"].(SettingsModelCatalog)
	require.Equal(t, []SettingsCatalogModel{{ModelKey: "coze/assistant:custom:latest", Provider: "coze", Model: "custom:latest", Label: "custom:latest"}}, models.Items)
	for _, tc := range []struct{ provider, app, code string }{
		{"not-in-settings", "", "AI_SETTINGS_PROVIDER_INVALID"},
		{"coze", "", "AI_SETTINGS_APP_REQUIRED"},
		{"coze", "unknown", "AI_SETTINGS_APP_INVALID"},
		{"ollama", "unknown", "AI_SETTINGS_APP_INVALID"},
	} {
		_, err := (&MediaInvoker{Service: f.s}).InvokeCoreCapability(f.ctx, settingsCatalogInput(MediaCatalogCapability, map[string]any{"operation": settingsModelsOperation, "modality": "llm", "provider": tc.provider, "app": tc.app}))
		requireMediaCode(t, err, tc.code)
	}
	doc, err := openapi3.NewLoader().LoadFromFile("../../../../docs/contracts/ai-media-v2.openapi.yaml")
	require.NoError(t, err)
	for _, request := range []map[string]any{body, modelsBody, {"operation": "list_models", "provider": "qwen-cn", "capability": catalog.ImageGeneration}} {
		require.NoError(t, doc.Components.Schemas["CatalogOperation"].Value.VisitJSON(request))
	}
	for name, result := range map[string]any{"SettingsProviderCatalog": out, "SettingsModelCatalog": models} {
		raw, err := json.Marshal(result)
		require.NoError(t, err)
		var object any
		require.NoError(t, json.Unmarshal(raw, &object))
		require.NoError(t, doc.Components.Schemas[name].Value.VisitJSON(object))
	}
	for _, request := range []map[string]any{
		{"operation": settingsProvidersOperation},
		{"operation": settingsProvidersOperation, "modality": "llm", "provider": ""},
		{"operation": settingsModelsOperation, "modality": "image", "provider": "ollama"},
		{"operation": "list_models", "modality": "llm"},
	} {
		require.Error(t, doc.Components.Schemas["CatalogOperation"].Value.VisitJSON(request))
	}
	require.Zero(t, f.generations.Load())
}

func TestSettingsCatalogRejectsOverridesAndWrongCapability(t *testing.T) {
	f := newMediaFixture(t)
	i := &MediaInvoker{Service: f.s}
	for _, key := range []string{"env", "base_url", "baseURL", "endpoint", "method", "headers", "api_key", "apiKey", "context", "capability", "provider", "app"} {
		body := map[string]any{"operation": settingsProvidersOperation, "modality": "llm", key: ""}
		_, err := i.InvokeCoreCapability(f.ctx, settingsCatalogInput(settingsProvidersCapability, body))
		requireMediaCode(t, err, "AI_OPERATION_INVALID")
	}
	for _, tc := range []struct {
		id   string
		body map[string]any
	}{
		{settingsProvidersCapability, map[string]any{"operation": settingsModelsOperation, "modality": "llm", "provider": "ollama"}},
		{MediaCatalogCapability, map[string]any{"operation": settingsProvidersOperation, "modality": "llm"}},
		{settingsProvidersCapability, map[string]any{"operation": settingsProvidersOperation}},
		{settingsProvidersCapability, map[string]any{"operation": settingsProvidersOperation, "modality": "image"}},
		{MediaCatalogCapability, map[string]any{"operation": "list_models", "modality": "llm"}},
	} {
		_, err := i.InvokeCoreCapability(f.ctx, settingsCatalogInput(tc.id, tc.body))
		requireMediaCode(t, err, "AI_OPERATION_INVALID")
	}
	body := map[string]any{"operation": settingsProvidersOperation, "modality": "llm"}
	for _, key := range []string{"endpoint", "method", "headers", "context", "env"} {
		input := settingsCatalogInput(settingsProvidersCapability, body)
		input.Payload[key] = ""
		_, err := i.InvokeCoreCapability(f.ctx, input)
		requireMediaCode(t, err, "AI_TYPED_OPERATION_REQUIRED")
	}
	require.NoError(t, f.db.Where("tenant_uuid = ? AND key = ?", mediaTestTenant, "ai.current_env").Delete(&setting.TenantSetting{}).Error)
	_, err := i.InvokeCoreCapability(f.ctx, settingsCatalogInput(settingsProvidersCapability, body))
	requireMediaCode(t, err, "AI_ENV_NOT_CONFIGURED")
	require.Zero(t, f.generations.Load())
}

func TestSettingsCatalogOllamaUsesOnlySavedTenantEnvironmentConnection(t *testing.T) {
	f := newMediaFixture(t)
	var defaultCalls atomic.Int32
	defaultInstance := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defaultCalls.Add(1)
		if r.URL.Path == "/api/tags" {
			_, _ = w.Write([]byte(`{"models":[{"name":"staging-default:14b"}]}`))
		} else {
			_, _ = w.Write([]byte(`{"capabilities":["completion"]}`))
		}
	}))
	defer defaultInstance.Close()
	manifest, _ := catalog.GetGlobalAIRegister().Manifest("ollama")
	manifest.Auth.Defaults["base_url"] = defaultInstance.URL
	var status atomic.Int32
	status.Store(http.StatusOK)
	var empty atomic.Bool
	instance := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(int(status.Load()))
		switch r.URL.Path {
		case "/api/tags":
			require.Equal(t, http.MethodGet, r.Method)
			if empty.Load() {
				_, _ = w.Write([]byte(`{"models":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"models":[{"name":"private-instance:14b"},{"name":"qwen3:14b"},{"name":"vector:latest"}]}`))
		case "/api/show":
			require.Equal(t, http.MethodPost, r.Method)
			var body map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			capabilities := []string{"completion"}
			if body["model"] == "vector:latest" {
				capabilities = []string{"embedding"}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": capabilities})
		default:
			t.Errorf("unexpected provider request %s", r.URL.Path)
		}
	}))
	defer instance.Close()
	save := func(environment, tenant, address string) {
		require.NoError(t, f.s.ai.settings.SaveCredentialOnly(f.ctx, environment, &tenant, &am.AIProviderCredential{Name: environment + "-ollama", Provider: "ollama", Data: datatypes.JSONMap{"base_url": address, "api_key": "never-output-this-secret"}}))
	}
	save("dev", "22222222-2222-4222-8222-222222222222", instance.URL)
	save("staging", mediaTestTenant, defaultInstance.URL)
	body := map[string]any{"operation": settingsModelsOperation, "modality": "llm", "provider": "ollama", "app": ""}
	i := &MediaInvoker{Service: f.s}
	_, err := i.InvokeCoreCapability(f.ctx, settingsCatalogInput(MediaCatalogCapability, body))
	requireMediaCode(t, err, "AI_SETTINGS_PROVIDER_NOT_CONFIGURED")
	require.Zero(t, defaultCalls.Load(), "an unconfigured tenant must not probe the Core host")
	save("dev", mediaTestTenant, instance.URL)
	out := invokeSettingsCatalog(t, f, MediaCatalogCapability, body)["result"].(SettingsModelCatalog)
	want, err := f.s.ai.settings.ModelsForTenant(f.ctx, "dev", ptr(mediaTestTenant), "llm", "ollama", "", instance.URL)
	require.NoError(t, err)
	actual := []string{}
	for _, model := range out.Items {
		actual = append(actual, model.Model)
	}
	require.Equal(t, []string{"private-instance:14b", "qwen3:14b"}, actual)
	require.Equal(t, want, actual)
	require.Equal(t, "ollama/private-instance:14b", out.Items[0].ModelKey)
	raw, err := json.Marshal(out)
	require.NoError(t, err)
	for _, secret := range []string{instance.URL, "base_url", "api_key", "never-output-this-secret"} {
		require.NotContains(t, string(raw), secret)
	}
	status.Store(http.StatusServiceUnavailable)
	_, err = i.InvokeCoreCapability(f.ctx, settingsCatalogInput(MediaCatalogCapability, body))
	requireMediaCode(t, err, "AI_MODEL_DISCOVERY_UPSTREAM_FAILED")
	status.Store(http.StatusOK)
	empty.Store(true)
	out = invokeSettingsCatalog(t, f, MediaCatalogCapability, body)["result"].(SettingsModelCatalog)
	require.NotNil(t, out.Items)
	require.Empty(t, out.Items)
	// 保存过的默认连接允许使用 Core 默认地址；缺配置时上面已验证不会回退。
	require.NoError(t, f.db.Model(&am.AIProviderCredential{}).Where("env = ? AND tenant_uuid = ? AND provider = ?", "dev", mediaTestTenant, "ollama").Update("data", datatypes.JSONMap{"base_url": ""}).Error)
	out = invokeSettingsCatalog(t, f, MediaCatalogCapability, body)["result"].(SettingsModelCatalog)
	require.Equal(t, "staging-default:14b", out.Items[0].Model)
	require.NoError(t, f.s.ai.settings.SetTenantCurrentAIEnv(context.Background(), mediaTestTenant, "staging"))
	out = invokeSettingsCatalog(t, f, MediaCatalogCapability, body)["result"].(SettingsModelCatalog)
	require.Equal(t, "staging", out.Env)
	require.Equal(t, "staging-default:14b", out.Items[0].Model)
	require.Zero(t, f.generations.Load())
}
