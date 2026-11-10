package agent

import (
	"context"
	"encoding/json"
	am "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSyncedParametersSurviveReadbackWithoutChangingUserProfiles(t *testing.T) {
	s, _ := onlineCatalogFixture(t)
	ctx := context.Background()
	tenant := onlineTestTenant
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-6-luna"},{"id":"unknown-native"}]}`))
	}))
	defer srv.Close()
	savedOnlineCredential(t, s, tenant, "dev", "openai", srv.URL+"/v1")
	saved := datatypes.JSONMap{"temperature": float64(0), "maxTokens": float64(777), "topP": .8, "reasoning_effort": "none"}
	require.NoError(t, s.db.Create(&am.AIModelProfile{Env: "dev", TenantUUID: &tenant, Modality: "llm", Provider: "openai", Model: "gpt-6-luna", Defaults: saved}).Error)
	request := OnlineModelSyncRequest{Env: "dev", Provider: "openai", Modality: "llm", Action: "refresh"}
	synced, err := s.SyncOnlineModels(ctx, tenant, request)
	require.NoError(t, err)
	require.Equal(t, "medium", synced.Models[0].ParameterProfile.Defaults["reasoning_effort"])
	fresh := NewAgentSettingService(s.db)
	metadata := fresh.ModelParameterProfile(ctx, "dev", &tenant, "openai", "gpt-6-luna")
	require.Equal(t, "official_docs", metadata.Source)
	require.Equal(t, "unknown", fresh.ModelParameterProfile(ctx, "dev", &tenant, "openai", "unknown-native").Source)
	mc, err := fresh.BuildModelConfig(ctx, "dev", &tenant, "llm", "openai", "gpt-6-luna", "", "", "", "", "", "")
	require.NoError(t, err)
	require.Equal(t, metadata, mc.ParameterProfile)
	_, err = s.SyncOnlineModels(ctx, tenant, request)
	require.NoError(t, err)
	var user am.AIModelProfile
	require.NoError(t, s.db.Where("env = ? AND tenant_uuid = ? AND provider = ? AND model = ?", "dev", tenant, "openai", "gpt-6-luna").First(&user).Error)
	want, _ := json.Marshal(saved)
	actual, _ := json.Marshal(user.Defaults)
	require.JSONEq(t, string(want), string(actual))
	out := &dto.ChatConfig{}
	applyLLMDefaultsFromProfile(out, &user)
	require.True(t, out.TemperatureSet)
	require.EqualValues(t, 0, out.Temperature)
	require.Equal(t, "none", out.ReasoningEffort)
	require.InDelta(t, .8, out.TopP, .0001)
	require.Equal(t, 2, calls, "读取参数及构造运行时配置不应在线探测")
}

func TestRuntimeModelSwitchLoadsItsOwnSavedDefaults(t *testing.T) {
	s, _ := onlineCatalogFixture(t)
	tenant := onlineTestTenant
	require.NoError(t, s.db.Create(&am.AIModelProfile{Env: "dev", TenantUUID: &tenant, Modality: "llm", Provider: "openai", Model: "gpt-6-luna", Defaults: datatypes.JSONMap{"maxTokens": 777, "topP": .8, "temperature": 0, "reasoning_effort": "none"}}).Error)
	resolver := &ChatConfigResolver{setting: s}
	out := &dto.ChatConfig{Provider: "openai", ModelName: "gpt-4o", MaxTokens: 4096, Temperature: .7, SystemPrompt: "agent persona"}
	require.NoError(t, resolver.mergeModelOverride(context.Background(), "dev", &tenant, out, &dto.ChatConfig{ModelName: "gpt-6-luna"}))
	require.Equal(t, 777, out.MaxTokens)
	require.Equal(t, "none", out.ReasoningEffort)
	require.True(t, out.TemperatureSet)
	require.Zero(t, out.Temperature)
	require.Equal(t, "agent persona", out.SystemPrompt)
	require.NoError(t, resolver.mergeModelOverride(context.Background(), "dev", &tenant, out, &dto.ChatConfig{MaxTokens: 555, TopP: .5}))
	require.Equal(t, 555, out.MaxTokens)
	require.EqualValues(t, .5, out.TopP)
	require.NoError(t, resolver.mergeModelOverride(context.Background(), "dev", &tenant, out, &dto.ChatConfig{Provider: "qwen-cn", ModelName: "qwen-plus"}))
	require.Empty(t, out.APIKey)
	require.Empty(t, out.Endpoint)
	require.Empty(t, out.ReasoningEffort)
	require.Zero(t, out.MaxTokens)
}
