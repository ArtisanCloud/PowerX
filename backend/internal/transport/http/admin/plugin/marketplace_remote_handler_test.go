package plugin

import (
	"context"
	"encoding/json"
	"errors"
	mgrimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	market "github.com/ArtisanCloud/PowerX/internal/infra/plugin/marketplace"
	marketsvc "github.com/ArtisanCloud/PowerX/internal/service/plugin_marketplace"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRemoteMarketplaceRoutesRequireRoot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	deps := &shared.Deps{PluginMarketplace: marketsvc.NewService(market.NewClient("", "never-expose-this", http.DefaultClient), "1.0.0", nil)}
	r.GET("/catalog", pluginRootOnlyMiddleware(), MarketplaceCatalogHandler(deps))
	r.GET("/catalog/:plugin_id/versions", pluginRootOnlyMiddleware(), MarketplaceVersionsHandler(deps))
	r.POST("/install", pluginRootOnlyMiddleware(), MarketplaceInstallHandler(deps))
	for _, req := range []*http.Request{httptest.NewRequest("GET", "/catalog", nil), httptest.NewRequest("GET", "/catalog/com.test.demo/versions", nil), httptest.NewRequest("POST", "/install", nil)} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, 403, w.Code)
		require.NotContains(t, w.Body.String(), "never-expose-this")
	}
}

func TestUnconfiguredMarketplaceHasExplicitConnectionState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	deps := &shared.Deps{PluginMarketplace: marketsvc.NewService(market.NewClient("", "", http.DefaultClient), "1.0.0", nil)}
	r.GET("/catalog", MarketplaceCatalogHandler(deps))
	req := httptest.NewRequest("GET", "/catalog", nil)
	req = req.WithContext(reqctx.WithIsRoot(req.Context(), true))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
	var result struct {
		Data struct {
			Connection struct {
				Configured    bool `json:"configured"`
				DownloadReady bool `json:"download_ready"`
			} `json:"connection"`
			Items []market.Plugin `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	require.False(t, result.Data.Connection.Configured)
	require.False(t, result.Data.Connection.DownloadReady)
	require.NotNil(t, result.Data.Items)
	require.Empty(t, result.Data.Items)
}

func TestMarketDownloadErrorsNeverExposeSignedURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	respondMarketError(c, plugin_mgr.Wrap(plugin_mgr.CodeIOError, errors.New("GET http://storage/object?X-Amz-Signature=private-secret failed")))
	require.Equal(t, 502, w.Code)
	require.NotContains(t, w.Body.String(), "private-secret")
	require.NotContains(t, w.Body.String(), "X-Amz")
}

type marketInstallManager struct {
	plugin_mgr.Manager
	current *plugin_mgr.Plugin
	options plugin_mgr.InstallOptions
	calls   int
}

func (m *marketInstallManager) Get(context.Context, string) (plugin_mgr.Plugin, error) {
	if m.current == nil {
		return plugin_mgr.Plugin{}, plugin_mgr.NewError(plugin_mgr.CodeNotFound)
	}
	return *m.current, nil
}
func (m *marketInstallManager) InstallFromURL(_ context.Context, url, sum, signature string, opts plugin_mgr.InstallOptions) (plugin_mgr.Plugin, error) {
	m.calls++
	m.options = opts
	state := plugin_mgr.StateInstalled
	if opts.AutoEnable {
		state = plugin_mgr.StateEnabled
	}
	return plugin_mgr.Plugin{ID: "com.test.demo", Version: "1.2.0", Provider: opts.Provider, Revision: "new-revision", State: state, InstallOrigin: opts.Origin}, nil
}
func TestMarketplaceInstallUsesVerifiedPublisherAndSharedInstallerOptions(t *testing.T) {
	for _, mode := range []string{"first", "enable_poststep_failure", "replace", "legacy_binding", "unconfirmed", "different_provider", "stale_revision", "missing_confirmation"} {
		t.Run(mode, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			downloads := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/plugins/com.test.demo":
					require.Empty(t, r.Header.Get("Authorization"))
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": market.Plugin{ID: "com.test.demo", OwnerDevID: "catalog-developer"}})
				case "/api/v1/plugins/com.test.demo/versions":
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": market.Page[market.Version]{Items: []market.Version{{PluginID: "com.test.demo", Semver: "1.2.0", MinCore: "1.0.0", Target: market.Target{OS: runtime.GOOS, Arch: runtime.GOARCH, SHA256: strings.Repeat("a", 64), Size: 123}}}, Total: 1}})
				case "/api/v1/downloads":
					downloads++
					require.Equal(t, "Bearer private-token", r.Header.Get("Authorization"))
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": market.Download{SignedURL: "https://store.invalid/package?private-signature", ExpiresAt: time.Now().Add(time.Minute), Target: market.Target{OS: runtime.GOOS, Arch: runtime.GOARCH, SHA256: strings.Repeat("a", 64), Size: 123}}})
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			provider := plugin_mgr.ProviderIdentity{ID: "catalog-developer", Authority: server.URL + "/api/v1"}
			mgr := &marketInstallManager{}
			body := map[string]any{"plugin_id": "com.test.demo", "version": "1.2.0", "enable": false}
			expectedStatus := 200
			if mode != "first" && mode != "enable_poststep_failure" {
				mgr.current = &plugin_mgr.Plugin{ID: "com.test.demo", Version: "1.0.0", Revision: "old-revision", Provider: provider, InstallOrigin: plugin_mgr.InstallOrigin{Type: "local"}}
				body["replace"] = true
				body["expected_revision"] = "old-revision"
			}
			switch mode {
			case "enable_poststep_failure":
				body["enable"] = true
				expectedStatus = 500
			case "legacy_binding":
				mgr.current.Provider = plugin_mgr.ProviderIdentity{}
				body["confirm_legacy_provider"] = true
			case "unconfirmed":
				mgr.current.Provider = plugin_mgr.ProviderIdentity{}
				expectedStatus = 409
			case "different_provider":
				mgr.current.Provider.ID = "other-developer"
				body["confirm_legacy_provider"] = true
				expectedStatus = 409
			case "stale_revision":
				body["expected_revision"] = "stale"
				expectedStatus = 409
			case "missing_confirmation":
				delete(body, "replace")
				delete(body, "expected_revision")
				expectedStatus = 409
			}
			mgrimpl.ResetGlobalForTest(mgr)
			t.Cleanup(func() { mgrimpl.ResetGlobalForTest(nil) })
			deps := &shared.Deps{PluginMarketplace: marketsvc.NewService(market.NewClient(server.URL+"/api/v1", "private-token", server.Client()), "1.0.0", nil)}
			engine := gin.New()
			engine.POST("/install", MarketplaceInstallHandler(deps))
			raw, err := json.Marshal(body)
			require.NoError(t, err)
			request := httptest.NewRequest("POST", "/install", strings.NewReader(string(raw)))
			request.Header.Set("Content-Type", "application/json")
			request = request.WithContext(reqctx.WithTenantUUID(reqctx.WithIsRoot(request.Context(), true), "00000000-0000-4000-8000-000000000001"))
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			require.Equal(t, expectedStatus, response.Code, response.Body.String())
			require.NotContains(t, response.Body.String(), "private-token")
			require.NotContains(t, response.Body.String(), "private-signature")
			if expectedStatus == 409 {
				require.Zero(t, mgr.calls)
				require.Zero(t, downloads)
				return
			}
			require.Equal(t, 1, mgr.calls)
			require.Equal(t, 1, downloads)
			require.Equal(t, provider, mgr.options.Provider)
			require.True(t, mgr.options.VerifyChecksum)
			require.Equal(t, "com.test.demo", mgr.options.ExpectedPluginID)
			require.Equal(t, "1.2.0", mgr.options.ExpectedVersion)
			require.Equal(t, mode == "enable_poststep_failure", mgr.options.AutoEnable)
			require.Equal(t, mode == "replace" || mode == "legacy_binding", mgr.options.Force)
			require.Equal(t, mode == "legacy_binding", mgr.options.ConfirmLegacyProvider)
			if expectedStatus == 500 {
				require.Contains(t, response.Body.String(), "插件已安装")
			}
		})
	}
}
