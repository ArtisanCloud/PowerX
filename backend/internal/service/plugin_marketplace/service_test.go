package plugin_marketplace

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	market "github.com/ArtisanCloud/PowerX/internal/infra/plugin/marketplace"
	"github.com/stretchr/testify/require"
)

// 使用真实 HTTP 合同验证服务编排，防止目录身份或浏览器指定的平台替代宿主平台。
func TestResolveDownloadContract(t *testing.T) {
	for _, mode := range []string{"success", "unauthorized", "hash_changed", "target_changed", "expired", "missing_version", "invalid_hash", "core_too_old", "versions_error"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/api/v1/plugins/com.test.demo/versions" {
					require.Empty(t, r.Header.Get("Authorization"))
					require.Equal(t, runtime.GOOS, r.URL.Query().Get("os"))
					require.Equal(t, runtime.GOARCH, r.URL.Query().Get("arch"))
					if mode == "versions_error" {
						w.WriteHeader(500)
						_, _ = w.Write([]byte(`{"message":"database details and private storage key"}`))
						return
					}
					v := market.Version{PluginID: "com.test.demo", Semver: "1.2.0", MinCore: "1.0.0", Target: market.Target{OS: runtime.GOOS, Arch: runtime.GOARCH, SHA256: strings.Repeat("a", 64), Size: 123}}
					if mode == "missing_version" {
						v.Semver = "1.1.0"
					}
					if mode == "invalid_hash" {
						v.SHA256 = ""
					}
					if mode == "core_too_old" {
						v.MinCore = "2.0.0"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": market.Page[market.Version]{Items: []market.Version{v}, Total: 1, Page: 1, Size: 100}})
					return
				}
				require.Equal(t, "/api/v1/downloads", r.URL.Path)
				require.Equal(t, http.MethodPost, r.Method)
				require.Equal(t, "Bearer private-download-token", r.Header.Get("Authorization"))
				calls++
				var body map[string]string
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Equal(t, map[string]string{"plugin_id": "com.test.demo", "version": "1.2.0", "os": runtime.GOOS, "arch": runtime.GOARCH, "tenant_id": "tenant-test"}, body)
				if mode == "unauthorized" {
					w.WriteHeader(401)
					return
				}
				d := market.Download{SignedURL: server.URL + "/package.pxp?signature=private", ExpiresAt: time.Now().Add(time.Minute), Target: market.Target{OS: runtime.GOOS, Arch: runtime.GOARCH, SHA256: strings.Repeat("a", 64), Size: 123}}
				if mode == "hash_changed" {
					d.SHA256 = strings.Repeat("b", 64)
				}
				if mode == "target_changed" {
					d.OS = "wrong"
				}
				if mode == "expired" {
					d.ExpiresAt = time.Now().Add(-time.Minute)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": d})
			}))
			defer server.Close()
			svc := NewService(market.NewClient(server.URL+"/api/v1", "private-download-token", server.Client()), "v1.0.0", nil)
			_, err := svc.ResolveDownload(context.Background(), "com.test.demo", "1.2.0", "tenant-test")
			if mode == "success" {
				require.NoError(t, err)
				require.Equal(t, 1, calls)
			} else {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "private")
			}
			if mode == "missing_version" || mode == "invalid_hash" || mode == "core_too_old" || mode == "versions_error" {
				require.Zero(t, calls)
			}
		})
	}
}

func TestDownloadRequiresServerCredentialAndExplicitVersion(t *testing.T) {
	svc := NewService(market.NewClient("http://market.invalid/api/v1", "", http.DefaultClient), "1.0.0", nil)
	_, err := svc.ResolveDownload(context.Background(), "com.test.demo", "1.2.0", "tenant-test")
	require.ErrorContains(t, err, "下载凭证")
	for _, version := range []string{"latest", "", "../../package"} {
		_, err := svc.ResolveDownload(context.Background(), "com.test.demo", version, "tenant-test")
		require.Error(t, err)
	}
	require.False(t, svc.Status().DownloadReady)
}

func TestCatalogDoesNotReturnPrivateFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/plugins", r.URL.Path)
		require.Equal(t, "3", r.URL.Query().Get("page"))
		require.Equal(t, runtime.GOOS, r.URL.Query().Get("os"))
		_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":"com.test.demo","name":"Demo","storage_key":"private","token":"secret"}],"total":25,"page":3,"size":12}}`))
	}))
	defer server.Close()
	svc := NewService(market.NewClient(server.URL+"/api/v1", "secret", server.Client()), "1.0.0", nil)
	page, err := svc.List(context.Background(), 3, 12)
	require.NoError(t, err)
	require.Equal(t, int64(25), page.Total)
	b, _ := json.Marshal(page)
	require.NotContains(t, string(b), "private")
	require.NotContains(t, string(b), "secret")
	b, _ = json.Marshal(svc.Status())
	require.NotContains(t, string(b), "secret")
}

func TestProviderComesFromServerCatalogOwner(t *testing.T) {
	for _, mode := range []string{"valid", "missing_owner", "wrong_id"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/api/v1/plugins/com.test.demo", r.URL.Path)
				require.Empty(t, r.Header.Get("Authorization"))
				require.Equal(t, runtime.GOOS, r.URL.Query().Get("os"))
				require.Equal(t, runtime.GOARCH, r.URL.Query().Get("arch"))
				id, owner := "com.test.demo", "catalog-developer"
				if mode == "missing_owner" {
					owner = ""
				}
				if mode == "wrong_id" {
					id = "com.other"
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"id": id, "owner_dev_id": owner}})
			}))
			defer server.Close()
			svc := NewService(market.NewClient(server.URL+"/api/v1", "private-token", server.Client()), "1.0.0", nil)
			provider, err := svc.Provider(context.Background(), "com.test.demo")
			if mode == "valid" {
				require.NoError(t, err)
				require.Equal(t, "catalog-developer", provider.ID)
				require.Equal(t, server.URL+"/api/v1", provider.Authority)
			} else {
				require.Error(t, err)
				require.False(t, provider.Known())
			}
		})
	}
}
