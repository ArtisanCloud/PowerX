package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	am "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/modeldiscovery"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestSaveDynamicModelUsesSubmittedConnectionAtomically(t *testing.T) {
	for _, provider := range []string{"openai", "openai_compatible", "deepseek", "moonshot", "huggingface", "qwen-cn", "qwen-intl", "anthropic", "google", "ollama"} {
		for _, existing := range []bool{false, true} {
			t.Run(provider+map[bool]string{false: "/first", true: "/replace-invalid"}[existing], func(t *testing.T) {
				s := environmentAliasFixture(t)
				ctx := context.Background()
				tenant := onlineTestTenant
				if existing {
					savedOnlineCredential(t, s, tenant, "dev", provider, ":invalid-old-endpoint")
				}
				spec, _ := modeldiscovery.ProviderSpec(provider)
				model := "online-only-model"
				modality := "llm"
				if provider == "google" {
					modality = "image"
					model = "gemini-2.5-flash-image"
				}
				status := 200
				calls := 0
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if status != 200 {
						w.WriteHeader(status)
						return
					}
					switch spec.Protocol {
					case "gemini":
						_, _ = w.Write([]byte(`{"models":[{"name":"models/` + model + `","output_modalities":["image"]}]}`))
					case "ollama":
						if r.URL.Path == "/api/show" {
							_, _ = w.Write([]byte(`{"capabilities":["completion"]}`))
							return
						}
						_, _ = w.Write([]byte(`{"models":[{"name":"` + model + `"}]}`))
					case "dashscope":
						require.Equal(t, "/api/v1/models", r.URL.Path)
						_, _ = w.Write([]byte(`{"output":{"models":[{"model":"` + model + `","output_modalities":["text"]}]}}`))
					default:
						_, _ = w.Write([]byte(`{"data":[{"id":"` + model + `","output_modalities":["text"]}]}`))
					}
				}))
				defer srv.Close()
				endpoint := srv.URL + "/v1"
				if spec.Protocol == "dashscope" {
					endpoint = srv.URL + "/compatible-mode/v1"
				}
				if spec.Protocol == "gemini" {
					endpoint = srv.URL + "/v1beta"
				}
				credential := &am.AIProviderCredential{Name: utils.Slug("dev-" + provider), Provider: provider, Data: datatypes.JSONMap{"base_url": endpoint, "api_key": "new-fixture-key"}}
				if !existing && (provider == "openai_compatible" || provider == "ollama") {
					credential.Data["api_key"] = ""
				}
				profile := &am.AIModelProfile{Provider: provider, Modality: modality, Model: model}
				if existing {
					// Reproduce the browser flow: new connection discovery succeeds
					// while the old persisted endpoint remains invalid.
					_, err := s.SyncOnlineModels(ctx, tenant, OnlineModelSyncRequest{Env: "dev", Provider: provider, Modality: modality, Action: "refresh", Connection: &OnlineModelDraftConnection{BaseURL: endpoint, APIKey: "new-fixture-key"}})
					require.NoError(t, err)
				}

				if provider == "google" { // Discovery cannot create a missing executable image operation.
					require.ErrorContains(t, s.SaveCredentialAndProfile(ctx, "dev", &tenant, credential, profile, true), "AI_MODEL_CAPABILITY_UNSUPPORTED")
					var count int64
					require.NoError(t, s.db.Model(&am.AIModelProfile{}).Count(&count).Error)
					require.Zero(t, count)
					return
				}
				require.NoError(t, s.SaveCredentialAndProfile(ctx, "dev", &tenant, credential, profile, true))
				require.Positive(t, calls)
				saved, err := s.credRepo.FindByScopeNameProvider(ctx, "dev", &tenant, utils.Slug("dev-"+provider), provider)
				require.NoError(t, err)
				require.Equal(t, endpoint, saved.Data["base_url"])
				before, _ := json.Marshal(saved.Data)
				missing := &am.AIProviderCredential{Name: saved.Name, Provider: provider, Data: datatypes.JSONMap{"base_url": endpoint, "api_key": "new-fixture-key"}}
				require.Error(t, s.SaveCredentialAndProfile(ctx, "dev", &tenant, missing, &am.AIModelProfile{Provider: provider, Modality: modality, Model: "missing-provider-model"}, true))
				var profileCount int64
				require.NoError(t, s.db.Model(&am.AIModelProfile{}).Count(&profileCount).Error)
				require.EqualValues(t, 1, profileCount)
				status = 401
				bad := &am.AIProviderCredential{Name: saved.Name, Provider: provider, Data: datatypes.JSONMap{"base_url": endpoint, "api_key": "rejected-key"}}
				if provider == "ollama" {
					bad.Data["base_url"] = srv.URL + "/different-instance"
				}
				require.Error(t, s.SaveCredentialAndProfile(ctx, "dev", &tenant, bad, &am.AIModelProfile{Provider: provider, Modality: modality, Model: model}, true))
				after, err := s.credRepo.FindByScopeNameProvider(ctx, "dev", &tenant, saved.Name, provider)
				require.NoError(t, err)
				raw, _ := json.Marshal(after.Data)
				require.JSONEq(t, string(before), string(raw))
				_, err = s.GetProfile(ctx, "dev", &tenant, modality, provider, model)
				require.NoError(t, err)
				require.NotNil(t, catalog.GetGlobalAIRegister())
			})
		}
	}
}
