package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	manager "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	market "github.com/ArtisanCloud/PowerX/internal/infra/plugin/marketplace"
	marketsvc "github.com/ArtisanCloud/PowerX/internal/service/plugin_marketplace"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type trackedMarketInstallManager struct {
	marketInstallManager
	fail         bool
	afterInstall func()
	enableCalls  int
}

func (m *trackedMarketInstallManager) Enable(context.Context, string) error {
	m.enableCalls++
	return nil
}

func (m *trackedMarketInstallManager) InstallFromURL(ctx context.Context, url, sum, sig string, opts plugin_mgr.InstallOptions) (plugin_mgr.Plugin, error) {
	p, err := m.marketInstallManager.InstallFromURL(ctx, url, sum, sig, opts)
	if err != nil {
		return p, err
	}
	if opts.DownloadVerified != nil {
		if err := opts.DownloadVerified(ctx, plugin_mgr.DownloadVerification{SHA256: sum, Size: opts.ExpectedSize}); err != nil {
			return plugin_mgr.Plugin{}, err
		}
	}
	if m.fail {
		return plugin_mgr.Plugin{}, errors.New("controlled install failure")
	}
	if m.afterInstall != nil {
		m.afterInstall()
	}
	return p, nil
}
func TestMarketplaceHTTPReportsActualInstallAndEnableOutcome(t *testing.T) {
	for _, mode := range []string{"install_only", "install_failure", "enable_failure", "root_without_tenant", "journal_install_only", "journal_enable_failure"} {
		t.Run(mode, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			var events []market.TransferReport
			downloads := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				respond := func(v any) { _ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": v}) }
				switch {
				case strings.HasSuffix(r.URL.Path, "/heartbeat"):
					respond(map[string]string{"status": "recorded"})
				case strings.HasSuffix(r.URL.Path, "/register"):
					respond(map[string]any{"instance": map[string]string{"id": uuid.NewString()}, "instance_token": "pxm_instance_private"})
				case r.URL.Path == "/api/v1/plugins/com.test.demo":
					respond(market.Plugin{ID: "com.test.demo", OwnerDevID: "developer-test"})
				case strings.HasSuffix(r.URL.Path, "/versions"):
					respond(market.Page[market.Version]{Items: []market.Version{{PluginID: "com.test.demo", Semver: "1.2.0", Target: market.Target{OS: runtime.GOOS, Arch: runtime.GOARCH, SHA256: strings.Repeat("a", 64), Size: 123}}}, Total: 1})
				case r.URL.Path == "/api/v1/downloads":
					downloads++
					require.Equal(t, "Bearer pxm_instance_private", r.Header.Get("Authorization"))
					respond(market.Download{DownloadID: uuid.NewString(), TrackingMode: "registered_instance", Status: "url_issued", SignedURL: "https://objects.invalid/package?private-signature", ExpiresAt: time.Now().Add(time.Minute), Target: market.Target{OS: runtime.GOOS, Arch: runtime.GOARCH, SHA256: strings.Repeat("a", 64), Size: 123}})
				case strings.HasSuffix(r.URL.Path, "/events"):
					var e market.TransferReport
					require.NoError(t, json.NewDecoder(r.Body).Decode(&e))
					events = append(events, e)
					respond(map[string]string{"status": e.Status})
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			client := market.NewClient(server.URL+"/api/v1", "", server.Client())
			directory := t.TempDir()
			store, err := market.OpenDistributionStore(directory, client.SourceID(), "instance-a")
			require.NoError(t, err)
			svc := marketsvc.NewService(client, "1.0.0", nil)
			svc.ConfigureDistribution(store, "pxm_enroll_private", time.Minute, nil)
			defer svc.CloseDistribution()
			require.NoError(t, svc.ConnectInstance(context.Background()))
			mgr := &trackedMarketInstallManager{fail: mode == "install_failure"}
			var restoreJournal func()
			if strings.HasPrefix(mode, "journal_") {
				mgr.afterInstall = func() {
					files, err := filepath.Glob(filepath.Join(directory, "*", "distribution.json"))
					require.NoError(t, err)
					require.Len(t, files, 1)
					file := files[0]
					require.NoError(t, os.Rename(file, file+".held"))
					require.NoError(t, os.Mkdir(file, 0o700))
					restoreJournal = func() { require.NoError(t, os.Remove(file)); require.NoError(t, os.Rename(file+".held", file)) }
				}
			}
			manager.ResetGlobalForTest(mgr)
			t.Cleanup(func() { manager.ResetGlobalForTest(nil) })
			r := gin.New()
			r.POST("/install", MarketplaceInstallHandler(&shared.Deps{PluginMarketplace: svc}))
			body := `{"plugin_id":"com.test.demo","version":"1.2.0","enable":false}`
			if mode == "enable_failure" || mode == "journal_enable_failure" {
				body = `{"plugin_id":"com.test.demo","version":"1.2.0","enable":true}`
			}
			req := httptest.NewRequest("POST", "/install", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(reqctx.WithTenantUUID(reqctx.WithIsRoot(req.Context(), true), "00000000-0000-4000-8000-000000000001"))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if mode == "install_only" || mode == "root_without_tenant" || mode == "journal_install_only" {
				require.Equal(t, 200, w.Code)
			} else {
				require.GreaterOrEqual(t, w.Code, 400)
			}
			require.NotContains(t, w.Body.String(), "private")
			if restoreJournal != nil {
				require.Equal(t, 1, svc.Status().Instance.UnconfirmedInstalls)
				restoreJournal()
				svc.StartDistribution(context.Background())
				require.Eventually(t, func() bool {
					status := svc.Status().Instance
					return status.UnconfirmedInstalls == 0 && status.PendingReports == 0
				}, 8*time.Second, 20*time.Millisecond)
				svc.CloseDistribution()
			}
			require.NoError(t, svc.FlushDistribution(context.Background()))
			require.Equal(t, 1, downloads)
			require.Equal(t, 1, mgr.calls)
			if mode == "install_only" || mode == "root_without_tenant" || mode == "journal_install_only" {
				require.Equal(t, []string{"downloaded", "installed"}, []string{events[0].Status, events[1].Status})
			} else if mode == "install_failure" {
				require.Len(t, events, 2)
				require.Equal(t, "failed", events[1].Status)
				require.Equal(t, "install", events[1].Stage)
			} else {
				require.Len(t, events, 3)
				require.Equal(t, "installed", events[1].Status)
				require.Equal(t, "failed", events[2].Status)
				require.Equal(t, "enable", events[2].Stage)
			}
			if mode == "root_without_tenant" {
				mgr.current = &plugin_mgr.Plugin{ID: "com.test.demo", Version: "1.2.0", Revision: "new-revision", InstallOrigin: svc.Origin()}
				r.POST("/plugins/:id/enable", PluginEnableHandler(&shared.Deps{PluginMarketplace: svc}))
				request := httptest.NewRequest("POST", "/plugins/com.test.demo/enable", nil)
				request = request.WithContext(reqctx.WithIsRoot(request.Context(), true))
				response := httptest.NewRecorder()
				r.ServeHTTP(response, request)
				require.Equal(t, 200, response.Code)
				require.Equal(t, 1, mgr.enableCalls)
				for _, a := range store.Snapshot().Attempts {
					require.Equal(t, "installed", a.Phase)
					require.Len(t, a.Events, 2)
				}
			}
			require.NoError(t, svc.FlushDistribution(context.Background()))
			require.Equal(t, 1, mgr.calls)
		})
	}
}
