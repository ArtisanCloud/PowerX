package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	am "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestClearingProviderEndpointTestsAndSavesDefaultWithoutReusingOldProxy(t *testing.T) {
	s := environmentAliasFixture(t)
	ctx, tenant := context.Background(), onlineTestTenant
	oldCalls, defaultCalls := 0, 0
	old := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { oldCalls++; w.WriteHeader(http.StatusUnauthorized) }))
	defer old.Close()
	current := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defaultCalls++
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/v1/models", r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer replacement-fixture-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o"}]}`))
	}))
	defer current.Close()
	manifest, _ := catalog.GetGlobalAIRegister().Manifest("openai")
	manifest.Auth.Defaults["base_url"] = current.URL + "/v1"
	savedOnlineCredential(t, s, tenant, "default", "openai", old.URL+"/v1")
	result, err := s.VerifyConnection(ctx, "dev", &tenant, contract.ModLLM, ConnectionInput{Provider: "openai", BaseURL: " ", APIKey: "replacement-fixture-key"})
	require.NoError(t, err)
	require.True(t, result.APIKeyVerified)
	require.Equal(t, 2, defaultCalls)
	require.Zero(t, oldCalls, "new credential must never be sent to the old proxy")
	// 尚未保存时服务调用仍读取原连接，不因管理表单临时输入而被改写。
	_, err = s.VerifySavedConnection(ctx, "dev", tenant, contract.ModLLM, "openai", "")
	require.ErrorContains(t, err, "AI_CONNECTION_AUTH_FAILED")
	require.Equal(t, 1, oldCalls)
	oldCalls = 0
	cred := &am.AIProviderCredential{Name: "dev-openai", Provider: "openai", Data: datatypes.JSONMap{"api_key": "replacement-fixture-key", "base_url": ""}}
	require.NoError(t, s.SaveCredentialOnly(ctx, "dev", &tenant, cred))
	stored, err := s.credRepo.FindByScopeNameProvider(ctx, "dev", &tenant, "dev-openai", "openai")
	require.NoError(t, err)
	require.Equal(t, "", stored.Data["base_url"])
	base, key, err := s.ResolveConnFromStore(ctx, "dev", &tenant, "openai", "", "")
	require.NoError(t, err)
	require.Equal(t, current.URL+"/v1", base)
	require.Equal(t, "replacement-fixture-key", key)
	_, err = s.VerifySavedConnection(ctx, "dev", tenant, contract.ModLLM, "openai", "")
	require.NoError(t, err)
	_, err = s.SyncOnlineModels(ctx, tenant, OnlineModelSyncRequest{Env: "dev", Provider: "openai", Modality: "llm", Action: "refresh"})
	require.NoError(t, err)
	require.Zero(t, oldCalls)
}

func TestEndpointClearPreservesStoredSecretButOmissionPreservesCustomEndpoint(t *testing.T) {
	for _, fullSave := range []bool{false, true} {
		t.Run(map[bool]string{false: "credential_only", true: "full_profile"}[fullSave], func(t *testing.T) {
			s := environmentAliasFixture(t)
			ctx, tenant := context.Background(), onlineTestTenant
			savedOnlineCredential(t, s, tenant, "default", "openai", "https://old-proxy.invalid/v1")
			require.NoError(t, s.SaveCredentialOnly(ctx, "dev", &tenant, &am.AIProviderCredential{Name: "dev-openai", Provider: "openai", Data: datatypes.JSONMap{}}))
			base, key, err := s.ResolveConnFromStore(ctx, "dev", &tenant, "openai", "", "")
			require.NoError(t, err)
			require.Equal(t, "https://old-proxy.invalid/v1", base)
			require.Equal(t, "fixture-key", key)
			cred := &am.AIProviderCredential{Name: "dev-openai", Provider: "openai", Data: datatypes.JSONMap{"base_url": ""}}
			if fullSave {
				err = s.SaveCredentialAndProfile(ctx, "dev", &tenant, cred, &am.AIModelProfile{Modality: "llm", Provider: "openai", Model: "gpt-4o"}, true)
			} else {
				err = s.SaveCredentialOnly(ctx, "dev", &tenant, cred)
			}
			require.NoError(t, err)
			base, key, err = s.ResolveConnFromStore(ctx, "dev", &tenant, "openai", "", "")
			require.NoError(t, err)
			require.Equal(t, "https://api.openai.com/v1", base)
			require.Equal(t, "fixture-key", key)
			legacy, err := s.credRepo.FindByScopeNameProvider(ctx, "default", &tenant, "default-openai", "openai")
			require.NoError(t, err)
			require.Equal(t, "https://old-proxy.invalid/v1", legacy.Data["base_url"])
		})
	}
}

func TestEndpointDefaultsFollowAuthModeAndNeverBorrowCustomURLWhenRequired(t *testing.T) {
	s := environmentAliasFixture(t)
	for _, tc := range []struct{ provider, mode, want string }{
		{"openai", "", "https://api.openai.com/v1"},
		{"hunyuan", "openai", "https://api.hunyuan.cloud.tencent.com/v1"},
		{"hunyuan", "tc3", "https://hunyuan.tencentcloudapi.com"},
		{"ollama", "", "http://127.0.0.1:11434"},
	} {
		base, err := ResolveFormBaseURL(tc.provider, tc.mode, "")
		require.NoError(t, err)
		require.Equal(t, tc.want, base)
	}
	tenant := onlineTestTenant
	savedOnlineCredential(t, s, tenant, "dev", "openai", "https://must-not-use.invalid/v1")
	manifest, _ := catalog.GetGlobalAIRegister().Manifest("openai")
	delete(manifest.Auth.Defaults, "base_url")
	err := s.SaveCredentialOnly(context.Background(), "dev", &tenant, &am.AIProviderCredential{Name: "dev-openai", Provider: "openai", Data: datatypes.JSONMap{"base_url": ""}})
	require.ErrorContains(t, err, "AI_CONNECTION_CONFIG_INVALID")
	_, err = s.VerifyConnection(context.Background(), "dev", &tenant, contract.ModLLM, ConnectionInput{Provider: "openai", APIKey: "test-key"})
	require.ErrorContains(t, err, "AI_CONNECTION_CONFIG_INVALID")
	stored, err := s.credRepo.FindByScopeNameProvider(context.Background(), "dev", &tenant, "dev-openai", "openai")
	require.NoError(t, err)
	require.Equal(t, "https://must-not-use.invalid/v1", stored.Data["base_url"])
}
