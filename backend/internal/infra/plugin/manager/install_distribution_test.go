package manager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

	coreconfig "github.com/ArtisanCloud/PowerX/config"
	market "github.com/ArtisanCloud/PowerX/internal/infra/plugin/marketplace"
	marketsvc "github.com/ArtisanCloud/PowerX/internal/service/plugin_marketplace"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestMarketplaceDistributionThroughRealInstaller(t *testing.T) {
	for _, mode := range []string{"installed", "checksum_failure", "journal_failure", "install_failure"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			src := writeReplaceTestPlugin(t, filepath.Join(root, "source"), "distribution-fixture")
			require.NoError(t, os.WriteFile(filepath.Join(src, "web-admin", ".output", "index.html"), []byte("fixture"), 0o644))
			payload := marketplacePackage(t, src, "zip")
			hash := sha256.Sum256(payload)
			sum := hex.EncodeToString(hash[:])
			target := market.Target{OS: runtime.GOOS, Arch: runtime.GOARCH, SHA256: sum, Size: int64(len(payload))}
			var statuses []string
			objectReads := 0
			token := "pxm_instance_test"
			instanceID := uuid.NewString()
			downloadID := uuid.NewString()
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				respond := func(v any) { _ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": v}) }
				if r.URL.Path == "/api/v1/host/instances/register" {
					respond(map[string]any{"instance": map[string]string{"id": instanceID}, "instance_token": token})
					return
				}
				if r.URL.Path == "/objects/package" {
					require.Empty(t, r.Header.Get("Authorization"))
					require.Empty(t, r.Header.Get("X-PowerX-Fingerprint"))
					objectReads++
					data := append([]byte{}, payload...)
					if mode == "checksum_failure" {
						data[len(data)-1] ^= 1
					}
					_, _ = w.Write(data)
					return
				}
				require.Equal(t, "Bearer "+token, r.Header.Get("Authorization"))
				require.Equal(t, market.Fingerprint("instance-a"), r.Header.Get("X-PowerX-Fingerprint"))
				switch {
				case strings.HasSuffix(r.URL.Path, "/versions"):
					respond(market.Page[market.Version]{Items: []market.Version{{PluginID: "com.powerx.plugins.replace-test", Semver: "0.1.0", MinCore: "1.0.0", Target: target}}, Total: 1})
				case r.URL.Path == "/api/v1/downloads":
					require.NotEmpty(t, r.Header.Get("Idempotency-Key"))
					respond(market.Download{DownloadID: downloadID, TrackingMode: "registered_instance", Status: "url_issued", SignedURL: server.URL + "/objects/package?secret", ExpiresAt: time.Now().Add(time.Minute), Target: target})
				case r.URL.Path == "/api/v1/downloads/"+downloadID+"/events":
					var event market.TransferReport
					require.NoError(t, json.NewDecoder(r.Body).Decode(&event))
					if event.Status == "downloaded" {
						require.Equal(t, sum, event.SHA256)
						require.Equal(t, int64(len(payload)), *event.Size)
					}
					statuses = append(statuses, event.Status)
					respond(map[string]string{"status": event.Status})
				default:
					t.Errorf("unexpected route %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			client := market.NewClient(server.URL+"/api/v1", "", server.Client())
			store, err := market.OpenDistributionStore(filepath.Join(root, "private"), client.SourceID(), "instance-a")
			require.NoError(t, err)
			svc := marketsvc.NewService(client, "1.0.0", nil)
			svc.ConfigureDistribution(store, "pxm_enroll_fixture", time.Minute, nil)
			defer svc.CloseDistribution()
			require.NoError(t, svc.ConnectInstance(ctx))
			attempt, err := svc.BeginInstall("com.powerx.plugins.replace-test", "0.1.0", "tenant-a", false, "")
			require.NoError(t, err)
			ticket, err := svc.ResolveDownload(ctx, "com.powerx.plugins.replace-test", "0.1.0", "tenant-a", attempt)
			require.NoError(t, err)
			reg := NewJSONRegistry(filepath.Join(root, "registry.json"))
			require.NoError(t, reg.Load(ctx))
			mgr := &managerImpl{opts: Options{InstalledRoot: filepath.Join(root, "installed"), Registry: reg, Loader: NewFSLoader(), CoreConfig: &coreconfig.Config{Deployment: coreconfig.DeploymentConfig{Env: coreconfig.DeploymentEnvDev}, EnvironmentID: "instance-a"}}, databaseSectionBuilder: testDatabaseSection}
			if mode == "install_failure" {
				mgr.opts.PostInstallManifest = func(context.Context, plugin_mgr.Manifest) error {
					return errors.New("controlled permission registration failure")
				}
			}
			if mode == "journal_failure" {
				require.NoError(t, store.Close())
			}
			p, err := mgr.InstallFromURL(ctx, ticket.SignedURL, ticket.SHA256, "", plugin_mgr.InstallOptions{Origin: svc.Origin(), VerifyChecksum: true, ExpectedPluginID: "com.powerx.plugins.replace-test", ExpectedVersion: "0.1.0", ExpectedSize: ticket.Size, DownloadVerified: func(_ context.Context, v plugin_mgr.DownloadVerification) error {
				require.Empty(t, reg.List(ctx))
				return svc.RecordDownloaded(attempt, v)
			}})
			if mode == "installed" {
				require.NoError(t, err)
				require.Len(t, reg.List(ctx), 1)
				require.NoError(t, svc.RecordInstalled(attempt, p))
				require.NoError(t, svc.FlushDistribution(ctx))
				require.Equal(t, []string{"downloaded", "installed"}, statuses)
			} else {
				require.Error(t, err)
				require.Empty(t, reg.List(ctx))
				if mode != "journal_failure" {
					require.NoError(t, svc.RecordFailed(attempt, "POWERX_INSTALL_FAILED"))
					require.NoError(t, svc.FlushDistribution(ctx))
					if mode == "checksum_failure" {
						require.Equal(t, []string{"failed"}, statuses)
					} else {
						require.Equal(t, []string{"downloaded", "failed"}, statuses)
					}
				}
			}
			require.Equal(t, 1, objectReads)
		})
	}
}
