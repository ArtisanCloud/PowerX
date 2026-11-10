package modeldiscovery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
)

func TestDiscoveryPreservesDeclaredMediaWithoutGuessingFromNames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"opaque-image","architecture":{"output_modalities":["image"]}},{"id":"opaque-audio","output_modalities":["audio"]},{"id":"opaque-video","output_modalities":["video"]},{"id":"gpt-image-future-without-metadata"}]}`))
	}))
	defer srv.Close()
	result, err := Fetch(context.Background(), Input{Provider: "openrouter", Endpoint: srv.URL, APIKey: "fixture-key"})
	require.NoError(t, err)
	for i, mod := range []string{"image", "audio", "video"} {
		require.Equal(t, []string{mod}, result.Models[i].Modalities)
	}
	require.Empty(t, result.Models[3].Modalities)
}

func TestProviderCatalogProtocolsPathsAuthenticationAndPagination(t *testing.T) {
	for _, tc := range []struct{ provider, base, path, protocol string }{
		{"openai", "/v1", "/v1/models", "openai"}, {"openrouter", "/api/v1", "/api/v1/models", "openai"}, {"deepseek", "", "/models", "openai"}, {"moonshot", "/v1", "/v1/models", "openai"}, {"huggingface", "/v1", "/v1/models", "openai"}, {"openai_compatible", "/proxy/v1", "/proxy/v1/models", "openai"},
		{"anthropic", "/proxy/v1", "/proxy/v1/models", "anthropic"}, {"google", "/v1beta", "/v1beta/models", "gemini"}, {"qwen-cn", "/compatible-mode/v1", "/api/v1/models", "dashscope"}, {"qwen-intl", "/gateway/compatible-mode/v1", "/gateway/api/v1/models", "dashscope"}, {"ollama", "/proxy/v1", "/proxy/api/tags", "ollama"},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Empty(t, r.URL.Query().Get("key"))
				require.NotContains(t, r.URL.String(), "fixture-key")
				switch tc.protocol {
				case "anthropic":
					require.Equal(t, "fixture-key", r.Header.Get("x-api-key"))
					require.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))
					require.Empty(t, r.Header.Get("Authorization"))
				case "gemini":
					require.Equal(t, "fixture-key", r.Header.Get("x-goog-api-key"))
				case "ollama":
					require.Empty(t, r.Header.Get("Authorization"))
				default:
					require.Equal(t, "Bearer fixture-key", r.Header.Get("Authorization"))
				}
				if tc.protocol == "ollama" && r.Method == http.MethodPost {
					require.Equal(t, "/proxy/api/show", r.URL.Path)
					var body map[string]string
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					require.Equal(t, "custom:14b", body["model"])
					_, _ = w.Write([]byte(`{"capabilities":["completion","vision"]}`))
					return
				}
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, tc.path, r.URL.Path)
				response := `{"data":[{"id":"new/model:tag","architecture":{"output_modalities":["text"]}}]}`
				switch tc.protocol {
				case "anthropic":
					if calls == 1 {
						response = `{"data":[{"id":"claude-new-1"}],"has_more":true,"last_id":"cursor-1"}`
					} else {
						require.Equal(t, "cursor-1", r.URL.Query().Get("after_id"))
						response = `{"data":[{"id":"claude-new-2"}],"has_more":false}`
					}
				case "gemini":
					if calls == 1 {
						response = `{"models":[{"name":"models/gemini-new","displayName":"New Gemini","supportedGenerationMethods":["generateContent"]}],"nextPageToken":"page-2"}`
					} else {
						require.Equal(t, "page-2", r.URL.Query().Get("pageToken"))
						response = `{"models":[{"name":"models/embed-new","supportedGenerationMethods":["embedContent"]}]}`
					}
				case "dashscope":
					if calls == 1 {
						require.Equal(t, "1", r.URL.Query().Get("page_no"))
						response = `{"success":true,"output":{"total":2,"models":[{"model":"qwen-new","capabilities":["TG"]}]}}`
					} else {
						require.Equal(t, "2", r.URL.Query().Get("page_no"))
						response = `{"success":true,"output":{"total":2,"models":[{"model":"qwen-image-new","capabilities":["IG"]}]}}`
					}
				case "ollama":
					response = `{"models":[{"name":"custom:14b"}]}`
				}
				if tc.provider == "openrouter" {
					require.Equal(t, "all", r.URL.Query().Get("output_modalities"))
				}
				_, _ = w.Write([]byte(response))
			}))
			defer srv.Close()
			key := "fixture-key"
			if tc.protocol == "ollama" {
				key = ""
			}
			result, err := Fetch(context.Background(), Input{Provider: tc.provider, Endpoint: srv.URL + tc.base, APIKey: key})
			require.NoError(t, err)
			require.Equal(t, tc.protocol, result.Protocol)
			require.Equal(t, srv.URL+tc.path, result.Source)
			require.Contains(t, result.Models[0].Modalities, "llm")
			if tc.protocol == "gemini" || tc.protocol == "dashscope" || tc.protocol == "anthropic" {
				require.Len(t, result.Models, 2)
				require.Equal(t, 2, calls)
			}
			if tc.protocol == "dashscope" {
				require.Equal(t, []string{"image"}, result.Models[1].Modalities, "preserve the explicit IG declaration without enabling a runtime contract")
			}
			if tc.protocol == "ollama" {
				require.Equal(t, "custom:14b", result.Models[0].ID)
				require.Equal(t, 2, calls)
			}
		})
	}
}

func TestDiscoveryFailsExplicitlyWithoutLeakingUpstreamBodiesOrFollowingRedirects(t *testing.T) {
	for _, tc := range []struct {
		status     int
		body, code string
	}{
		{401, "leaked-fixture-key", "AI_MODEL_SYNC_AUTH_FAILED"}, {500, "leaked-fixture-key", "AI_MODEL_DISCOVERY_UPSTREAM_FAILED"}, {200, `{"error":"leaked-fixture-key"}`, "AI_MODEL_DISCOVERY_RESPONSE_INVALID"}, {200, `{"data":[{}]}`, "AI_MODEL_DISCOVERY_RESPONSE_INVALID"}, {200, `{"data":[],"has_more":true}`, "AI_MODEL_DISCOVERY_RESPONSE_INVALID"},
	} {
		t.Run(tc.code+tc.body, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			result, err := Fetch(context.Background(), Input{Provider: "openai", Endpoint: srv.URL, APIKey: "fixture-key"})
			require.Nil(t, result)
			require.ErrorContains(t, err, tc.code)
			require.NotContains(t, err.Error(), "leaked-fixture-key")
		})
	}
	redirects := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirects++ }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer srv.Close()
	_, err := Fetch(context.Background(), Input{Provider: "openai", Endpoint: srv.URL, APIKey: "fixture-key"})
	require.ErrorContains(t, err, "AI_MODEL_DISCOVERY_UPSTREAM_FAILED")
	require.Zero(t, redirects)
	for _, endpoint := range []string{"file:///tmp/models", "https://user:secret@example.invalid", "https://example.invalid?key=secret", "https://example.invalid#fragment"} {
		_, err := Fetch(context.Background(), Input{Provider: "openai", Endpoint: endpoint, APIKey: "fixture-key"})
		require.ErrorContains(t, err, "AI_MODEL_DISCOVERY_CONFIG_INVALID")
	}
	for _, provider := range []string{"coze", "volcengine", "hunyuan", "hash", "agnes", "baidu", "comfyui", "sentence-transformers"} {
		_, err := Fetch(context.Background(), Input{Provider: provider})
		require.ErrorContains(t, err, "AI_MODEL_SYNC_UNSUPPORTED")
	}
	_, err = Fetch(context.Background(), Input{Provider: "openai", Endpoint: srv.URL})
	require.ErrorContains(t, err, "AI_SETTINGS_PROVIDER_NOT_CONFIGURED")
}

func TestDiscoveryRetiredFreeKeyReturnsOnlySafeDiagnosticClassification(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/models", r.URL.Path)
		require.Equal(t, "Bearer fixture-secret", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"新版本免费 API 已上线，老版本免费 API Key 已失效。请注册或登录并绑定 GitHub。当前请求使用的 ApiKey: fixture-secret","type":"chatanywhere_error","code":"403 FORBIDDEN"}}`))
	}))
	defer srv.Close()
	result, err := Fetch(context.Background(), Input{Provider: "openai", Endpoint: srv.URL, APIKey: "fixture-secret"})
	require.Nil(t, result)
	require.Equal(t, "AI_MODEL_SYNC_AUTH_FAILED", dto.CodeOf(err))
	require.Equal(t, map[string]any{"source": srv.URL + "/v1/models", "provider": "openai", "protocol": "openai", "upstream_status": 403, "upstream_reason": "legacy_free_key_retired"}, dto.DetailsOf(err))
	raw, marshalErr := json.Marshal(dto.DetailsOf(err))
	require.NoError(t, marshalErr)
	require.NotContains(t, string(raw)+err.Error(), "fixture-secret")
	for _, raw := range []string{`{"error":{"message":"Invalid key fixture-secret"}}`, strings.Repeat("x", 64<<10+1), `<html>forbidden fixture-secret</html>`} {
		require.Empty(t, authFailureReason(strings.NewReader(raw)))
	}
}

func TestDiscoveryEmptyAndUnknownModelsDoNotAcquireInventedCapabilities(t *testing.T) {
	response := `{"data":[]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(response)) }))
	defer srv.Close()
	result, err := Fetch(context.Background(), Input{Provider: "openai", Endpoint: srv.URL, APIKey: "fixture-key"})
	require.NoError(t, err)
	require.Empty(t, result.Models)
	response = `{"data":[{"id":"gpt-brand-new"},{"id":"gpt-brand-new"}]}`
	result, err = Fetch(context.Background(), Input{Provider: "openai", Endpoint: srv.URL, APIKey: "fixture-key"})
	require.NoError(t, err)
	require.Len(t, result.Models, 1)
	require.Empty(t, result.Models[0].Modalities)
	response = `{"data":[{"id":"` + strings.Repeat("x", 513) + `"}]}`
	_, err = Fetch(context.Background(), Input{Provider: "openai", Endpoint: srv.URL, APIKey: "fixture-key"})
	require.ErrorContains(t, err, "AI_MODEL_DISCOVERY_RESPONSE_INVALID")
}

func TestDiscoveryRespectsConfiguredMetadataBudget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer srv.Close()
	_, err := Fetch(context.Background(), Input{Provider: "openai", Endpoint: srv.URL, APIKey: "fixture-key", Timeout: 20 * time.Millisecond})
	require.ErrorContains(t, err, "AI_MODEL_DISCOVERY_TIMEOUT")
}
