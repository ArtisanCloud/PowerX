package manager

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
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
	"github.com/stretchr/testify/require"
)

func marketplacePackage(t *testing.T, src, format string) []byte {
	t.Helper()
	var buf bytes.Buffer
	var writer io.Closer
	var add func(string, os.FileInfo, []byte) error
	if format == "zip" {
		z := zip.NewWriter(&buf)
		writer = z
		add = func(name string, info os.FileInfo, data []byte) error {
			h, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			h.Name = "payload/" + name
			w, err := z.CreateHeader(h)
			if err != nil {
				return err
			}
			_, err = w.Write(data)
			return err
		}
	} else {
		gz := gzip.NewWriter(&buf)
		tr := tar.NewWriter(gz)
		add = func(name string, info os.FileInfo, data []byte) error {
			h, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			h.Name = "payload/" + name
			if err := tr.WriteHeader(h); err != nil {
				return err
			}
			_, err = tr.Write(data)
			return err
		}
		// gzip 必须在读取 buf 前关闭，由下面的显式回调负责。
		writer = &archiveCloser{first: tr, second: gz}
	}
	require.NoError(t, filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return add(filepath.ToSlash(rel), info, data)
	}))
	require.NoError(t, writer.Close())
	return buf.Bytes()
}

type archiveCloser struct{ first, second io.Closer }

func (c *archiveCloser) Close() error {
	if err := c.first.Close(); err != nil {
		return err
	}
	return c.second.Close()
}

func TestMarketplaceContractThroughInstaller(t *testing.T) {
	for _, format := range []string{"zip", "pxp"} {
		t.Run(format, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			src := writeReplaceTestPlugin(t, filepath.Join(root, "source"), "market-package")
			require.NoError(t, os.WriteFile(filepath.Join(src, "web-admin", ".output", "index.html"), []byte("test plugin UI"), 0o644))
			payload := marketplacePackage(t, src, format)
			hash := sha256.Sum256(payload)
			sum := hex.EncodeToString(hash[:])
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/plugins/com.powerx.plugins.replace-test/versions":
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": market.Page[market.Version]{Items: []market.Version{{PluginID: "com.powerx.plugins.replace-test", Semver: "0.1.0", MinCore: "1.0.0", Target: market.Target{OS: runtime.GOOS, Arch: runtime.GOARCH, SHA256: sum, Size: int64(len(payload))}}}, Total: 1}})
				case "/api/v1/downloads":
					require.Equal(t, "Bearer test-credential", r.Header.Get("Authorization"))
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": market.Download{SignedURL: server.URL + "/objects/package", ExpiresAt: time.Now().Add(time.Minute), Target: market.Target{OS: runtime.GOOS, Arch: runtime.GOARCH, SHA256: sum, Size: int64(len(payload))}}})
				case "/objects/package":
					require.Empty(t, r.Header.Get("Authorization"))
					_, _ = w.Write(payload)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			svc := marketsvc.NewService(market.NewClient(server.URL+"/api/v1", "test-credential", server.Client()), "1.0.0", nil)
			download, err := svc.ResolveDownload(ctx, "com.powerx.plugins.replace-test", "0.1.0", "tenant-test")
			require.NoError(t, err)
			reg := NewJSONRegistry(filepath.Join(root, "registry.json"))
			require.NoError(t, reg.Load(ctx))
			mgr := &managerImpl{opts: Options{InstalledRoot: filepath.Join(root, "installed"), Registry: reg, Loader: NewFSLoader(), CoreConfig: &coreconfig.Config{Deployment: coreconfig.DeploymentConfig{Env: coreconfig.DeploymentEnvDev}, EnvironmentID: "existing-environment-test"}}, databaseSectionBuilder: testDatabaseSection}
			opts := plugin_mgr.InstallOptions{Origin: svc.Origin(), VerifyChecksum: true, ExpectedPluginID: "com.powerx.plugins.replace-test", ExpectedVersion: "0.1.0", ExpectedSize: download.Size}
			// 校验失败与身份不符均不能登记插件或写入安装目录。
			_, err = mgr.InstallFromURL(ctx, download.SignedURL, strings.Repeat("0", 64), "", opts)
			require.Error(t, err)
			require.Empty(t, reg.List(ctx))
			bad := opts
			bad.ExpectedVersion = "9.9.9"
			_, err = mgr.InstallFromURL(ctx, download.SignedURL, sum, "", bad)
			require.Error(t, err)
			require.Empty(t, reg.List(ctx))
			_, err = os.Stat(filepath.Join(root, "installed"))
			require.True(t, os.IsNotExist(err))
			installed, err := mgr.InstallFromURL(ctx, download.SignedURL, sum, "", opts)
			require.NoError(t, err)
			require.Equal(t, "com.powerx.plugins.replace-test", installed.ID)
			require.Equal(t, "0.1.0", installed.Version)
			require.NotEqual(t, plugin_mgr.StateEnabled, installed.State)
			require.Len(t, reg.List(ctx), 1)
			require.Equal(t, svc.Origin().SourceID, installed.InstallOrigin.SourceID)
			require.Equal(t, sum, installed.InstallOrigin.SHA256)
			// 同一发布者允许通过显式替换改为本地来源；始终只有一份已安装版本。
			replaced, err := mgr.InstallFromFile(ctx, src, plugin_mgr.InstallOptions{Force: true, ExpectedRevision: installed.Revision})
			require.NoError(t, err)
			require.Equal(t, "local", replaced.InstallOrigin.Type)
			stored, ok := reg.Get(ctx, installed.ID)
			require.True(t, ok)
			require.Equal(t, replaced.InstallOrigin, stored.InstallOrigin)
			require.Len(t, reg.ListVersions(ctx, installed.ID), 1)
		})
	}
}

func TestRemoteTarRejectsTraversalAndLinks(t *testing.T) {
	for _, header := range []*tar.Header{
		{Name: "../escape", Typeflag: tar.TypeReg, Size: 1, Mode: 0o644},
		{Name: "payload/link", Typeflag: tar.TypeSymlink, Linkname: "../../outside"},
		{Name: "payload/link", Typeflag: tar.TypeLink, Linkname: "../../outside"},
	} {
		t.Run(header.Name+string(header.Typeflag), func(t *testing.T) {
			var buf bytes.Buffer
			gz := gzip.NewWriter(&buf)
			tr := tar.NewWriter(gz)
			require.NoError(t, tr.WriteHeader(header))
			if header.Size > 0 {
				_, err := tr.Write([]byte("x"))
				require.NoError(t, err)
			}
			require.NoError(t, tr.Close())
			require.NoError(t, gz.Close())
			root := t.TempDir()
			source := filepath.Join(root, "package.pxp")
			require.NoError(t, os.WriteFile(source, buf.Bytes(), 0o600))
			require.Error(t, extractRemotePackage(source, filepath.Join(root, "out")))
			_, err := os.Stat(filepath.Join(root, "escape"))
			require.True(t, os.IsNotExist(err))
		})
	}
}
