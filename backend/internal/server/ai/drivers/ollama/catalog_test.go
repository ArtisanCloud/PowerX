package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListModelsReadsInstanceAndFiltersExplicitCapabilities(t *testing.T) {
	for _, suffix := range []string{"", "/v1", "/api", "/gateway/v1/"} {
		t.Run(suffix, func(t *testing.T) {
			prefix := ""
			if suffix == "/gateway/v1/" {
				prefix = "/gateway"
			}
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case prefix + "/api/tags":
					require.Equal(t, http.MethodGet, r.Method)
					_, _ = w.Write([]byte(`{"models":[{"name":"custom:14b"},{"name":"not-named-embed:latest"},{"model":"vision:latest"},{"name":"custom:14b"}]}`))
				case prefix + "/api/show":
					require.Equal(t, http.MethodPost, r.Method)
					var body map[string]string
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					capabilities := map[string][]string{"custom:14b": {"completion"}, "not-named-embed:latest": {"embedding"}, "vision:latest": {"completion", "vision"}}
					require.Len(t, body, 1)
					require.Contains(t, capabilities, body["model"])
					_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": capabilities[body["model"]]})
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(500)
				}
			}))
			defer srv.Close()
			for modality, want := range map[string][]string{"llm": {"custom:14b", "vision:latest"}, "embedding": {"not-named-embed:latest"}, "image": {"vision:latest"}, "audio_tts": {}} {
				got, err := ListModels(context.Background(), srv.URL+suffix, modality)
				require.NoError(t, err)
				require.Equal(t, want, got)
			}
			require.Equal(t, 12, calls, "deduplicate tags before show; unsupported modalities make no requests")
		})
	}
}

func TestListModelsDistinguishesEmptyInvalidAndFailedInstances(t *testing.T) {
	for _, tc := range []struct {
		name, tags, show, code string
		status                 int
	}{
		{"empty", `{"models":[]}`, "", "", 200},
		{"invalid", `<html>private-diagnostic</html>`, "", "AI_MODEL_DISCOVERY_RESPONSE_INVALID", 200},
		{"missing models", `{}`, "", "AI_MODEL_DISCOVERY_RESPONSE_INVALID", 200},
		{"rejected", `private-diagnostic`, "", "AI_MODEL_DISCOVERY_UPSTREAM_FAILED", 401},
		{"no capabilities", `{"models":[{"name":"a:latest"}]}`, `{}`, "AI_MODEL_DISCOVERY_CAPABILITIES_UNAVAILABLE", 200},
		{"broken show", `{"models":[{"name":"a:latest"}]}`, `{`, "AI_MODEL_DISCOVERY_RESPONSE_INVALID", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				if r.URL.Path == "/api/show" {
					_, _ = w.Write([]byte(tc.show))
				} else {
					_, _ = w.Write([]byte(tc.tags))
				}
			}))
			defer srv.Close()
			got, err := ListModels(context.Background(), srv.URL, "llm")
			if tc.code == "" {
				require.NoError(t, err)
				require.Equal(t, []string{}, got)
			} else {
				require.ErrorContains(t, err, tc.code)
				require.NotContains(t, err.Error(), "private-diagnostic")
				require.Nil(t, got)
			}
		})
	}
}

func TestListModelsRejectsRedirectsAndInvalidEndpoints(t *testing.T) {
	hits := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer srv.Close()
	_, err := ListModels(context.Background(), srv.URL, "llm")
	require.ErrorContains(t, err, "AI_MODEL_DISCOVERY_UPSTREAM_FAILED")
	require.Zero(t, hits)
	for _, endpoint := range []string{"file:///etc/hosts", "http://user:secret@localhost", "http://localhost?api_key=secret", "http://localhost#fragment", "://"} {
		_, err := ListModels(context.Background(), endpoint, "llm")
		require.ErrorContains(t, err, "AI_MODEL_DISCOVERY_CONFIG_INVALID")
		require.NotContains(t, err.Error(), "secret")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = ListModels(ctx, srv.URL, "llm")
	require.ErrorContains(t, err, "AI_MODEL_DISCOVERY_TIMEOUT")
}

func TestListModelsSkipsDeletedOrRetiredModels(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusGone} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/tags" {
				_, _ = w.Write([]byte(`{"models":[{"name":"retired:cloud"},{"name":"local:14b"}]}`))
				return
			}
			var body map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			if body["model"] == "retired:cloud" {
				w.WriteHeader(status)
				return
			}
			_, _ = w.Write([]byte(`{"capabilities":["completion"]}`))
		}))
		got, err := ListModels(context.Background(), srv.URL, "llm")
		srv.Close()
		require.NoError(t, err)
		require.Equal(t, []string{"local:14b"}, got)
	}
}

func TestListModelsLiveReadOnly(t *testing.T) {
	endpoint := os.Getenv("POWERX_OLLAMA_CATALOG_LIVE_URL")
	if endpoint == "" {
		t.Skip("set POWERX_OLLAMA_CATALOG_LIVE_URL for read-only instance acceptance")
	}
	for _, modality := range []string{"llm", "embedding", "image"} {
		models, err := ListModels(context.Background(), endpoint, modality)
		require.NoError(t, err)
		require.NotNil(t, models)
		t.Logf("%s models: %v", modality, models)
	}
}
