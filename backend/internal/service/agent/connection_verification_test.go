package agent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSettingsConnectionOllamaAcceptsInstanceModelTags(t *testing.T) {
	previous := catalog.GlobalAIRegister
	reg := catalog.NewRegistry()
	require.NoError(t, reg.LoadByConfig(catalog.CatalogConfig{Dirs: []string{"../../../config/agents/providers.d"}, FailIfEmpty: true}, nil))
	catalog.GlobalAIRegister = reg
	t.Cleanup(func() { catalog.GlobalAIRegister = previous })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Close() })
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/tags", r.URL.Path)
		require.Empty(t, r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"models":[{"name":"qwen3:14b"},{"name":"bge-m3:latest"},{"name":"custom-vision:latest"}]}`))
	}))
	defer srv.Close()
	manifest, _ := reg.Manifest("ollama")
	manifest.Auth.Defaults["base_url"] = srv.URL
	svc := NewAgentSettingService(db)
	for _, tc := range []struct {
		modality contract.Modality
		model    string
	}{
		{contract.ModLLM, ""},
		{contract.ModLLM, "qwen3:14b"},
		{contract.ModEmbed, "bge-m3:latest"},
		{contract.ModImage, "custom-vision:latest"},
	} {
		for _, base := range []string{"", srv.URL + "/v1"} {
			result, err := svc.VerifyConnection(context.Background(), "dev", nil, tc.modality, ConnectionInput{Provider: "ollama", Model: tc.model, BaseURL: base})
			require.NoError(t, err, "dynamic model=%s base=%q", tc.model, base)
			require.True(t, result.EndpointReachable)
			require.Equal(t, "not_required", result.Authentication)
			require.False(t, result.APIKeyVerified)
			require.False(t, result.ModelAccessVerified)
			require.False(t, result.GenerationSubmitted)
		}
	}
	require.EqualValues(t, 8, calls.Load())
	for _, tc := range []struct{ modality, provider, model string }{
		{"llm", "ollama", ""},
		{"audio_tts", "ollama", "unregistered:latest"},
		{"llm", "unknown-provider", "qwen3:14b"},
		{"llm", "openai", "not-a-registered-model"},
	} {
		require.Error(t, ensureModelExists(tc.modality, tc.provider, tc.model))
	}
}

func TestSettingsConnectionOllamaLiveReadOnly(t *testing.T) {
	endpoint := os.Getenv("POWERX_OLLAMA_CATALOG_LIVE_URL")
	if endpoint == "" {
		t.Skip("set POWERX_OLLAMA_CATALOG_LIVE_URL for read-only connection acceptance")
	}
	previous := catalog.GlobalAIRegister
	reg := catalog.NewRegistry()
	require.NoError(t, reg.LoadByConfig(catalog.CatalogConfig{Dirs: []string{"../../../config/agents/providers.d"}, FailIfEmpty: true}, nil))
	catalog.GlobalAIRegister = reg
	t.Cleanup(func() { catalog.GlobalAIRegister = previous })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Close() })
	manifest, _ := reg.Manifest("ollama")
	manifest.Auth.Defaults["base_url"] = endpoint
	svc := NewAgentSettingService(db)
	for _, modality := range []contract.Modality{contract.ModLLM, contract.ModEmbed} {
		models, err := svc.ModelsForTenant(context.Background(), "dev", nil, string(modality), "ollama", "", endpoint)
		require.NoError(t, err)
		require.NotEmpty(t, models)
		for _, model := range []string{"", models[0]} {
			for _, base := range []string{"", endpoint} {
				result, err := svc.VerifyConnection(context.Background(), "dev", nil, modality, ConnectionInput{Provider: "ollama", Model: model, BaseURL: base})
				require.NoError(t, err)
				require.True(t, result.EndpointReachable)
				require.False(t, result.ModelAccessVerified)
				require.False(t, result.GenerationSubmitted)
				t.Logf("modality=%s model=%q default_address=%t endpoint_reachable=%t authentication=%s generation_submitted=%t", modality, model, base == "", result.EndpointReachable, result.Authentication, result.GenerationSubmitted)
			}
		}
	}
}

func TestSettingsConnectionNeverGenerates(t *testing.T) {
	previous := catalog.GlobalAIRegister
	reg := catalog.NewRegistry()
	require.NoError(t, reg.LoadByConfig(catalog.CatalogConfig{Dirs: []string{"../../../config/agents/providers.d"}, FailIfEmpty: true}, nil))
	catalog.GlobalAIRegister = reg
	t.Cleanup(func() { catalog.GlobalAIRegister = previous })
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Equal(t, "GET", r.Method)
		require.Equal(t, "/v1/models", r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(401)
			return
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()
	svc := NewAgentSettingService(nil)
	for _, tc := range []struct {
		provider, model string
		modality        contract.Modality
	}{
		{"agnes", "agnes-image-2.5-flash", contract.ModImage},
		{"agnes", "agnes-video-2.5", contract.ModVideo},
		{"openai", "gpt-4o-mini", contract.ModLLM},
		{"openai", "text-embedding-3-small", contract.ModEmbed},
		{"openai", "gpt-4o-mini-tts", contract.ModAudioTTS},
		{"openai", "whisper-1", contract.ModAudioASR},
		{"qwen-cn", "qwen-image-2.0-pro", contract.ModImage},
	} {
		t.Run(tc.provider+"/"+string(tc.modality), func(t *testing.T) {
			result, err := svc.VerifyConnection(context.Background(), "dev", nil, tc.modality, ConnectionInput{Provider: tc.provider, Model: tc.model, BaseURL: srv.URL + "/v1", APIKey: "test-key"})
			require.NoError(t, err)
			require.True(t, result.APIKeyVerified)
			require.False(t, result.GenerationSubmitted)
		})
	}
	require.EqualValues(t, 14, calls.Load())

	// 兼容方法也必须只读，不能绕回原来的图像／文本探针生成。
	require.NoError(t, svc.PingImage(context.Background(), "dev", nil, "agnes", "agnes-image-2.5-flash", srv.URL+"/v1", "test-key", "", "", "", ""))
	require.NoError(t, svc.PingLLM(context.Background(), "dev", nil, "openai", "gpt-4o-mini", srv.URL+"/v1", "test-key", "", "", "", ""))
	require.NoError(t, svc.TestConnectionPreferInput(context.Background(), "dev", nil, string(contract.ModVideo), "agnes", "agnes-video-2.5", srv.URL+"/v1", "test-key", "", "", "", ""))
	require.EqualValues(t, 20, calls.Load())

	// 无只读协议或错误目录配置，必须显式拒绝，不能降级为本地校验成功。
	for _, tc := range []struct {
		provider, model, auth, code string
		status                      int
	}{
		{"hunyuan", "hunyuan-lite", "tc3", "AI_CONNECTION_PROBE_UNSUPPORTED", 501},
	} {
		_, err := svc.VerifyConnection(context.Background(), "dev", nil, contract.ModLLM, ConnectionInput{Provider: tc.provider, Model: tc.model, BaseURL: srv.URL, APIKey: "test-key", AuthMode: tc.auth})
		var app *dto.AppError
		require.True(t, errors.As(err, &app))
		require.Equal(t, tc.status, app.HTTPCode)
		require.Equal(t, tc.code, app.Code)
	}
	require.EqualValues(t, 20, calls.Load())

	// 首次配置时未填可选入口：没有历史凭据也能使用供应商正式默认入口。
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	defer pool.Close()
	manifest, _ := reg.Manifest("agnes")
	manifest.Auth.Defaults["base_url"] = srv.URL + "/v1"
	result, err := NewAgentSettingService(db).VerifyConnection(context.Background(), "dev", nil, contract.ModImage, ConnectionInput{Provider: "agnes", Model: "agnes-image-2.5-flash", APIKey: "test-key"})
	require.NoError(t, err)
	require.True(t, result.APIKeyVerified)
	require.EqualValues(t, 22, calls.Load())
}
