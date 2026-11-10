package plugin

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	mgrimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type localUploadCompatibilityManager struct {
	plugin_mgr.Manager
	options plugin_mgr.InstallOptions
	source  string
	calls   int
	t       *testing.T
}

func (m *localUploadCompatibilityManager) InstallFromFile(_ context.Context, src string, opts plugin_mgr.InstallOptions) (plugin_mgr.Plugin, error) {
	m.calls++
	m.source = src
	m.options = opts
	raw, err := os.ReadFile(filepath.Join(src, "plugin.yaml"))
	require.NoError(m.t, err)
	require.Contains(m.t, string(raw), "com.test.upload")
	require.NotContains(m.t, string(raw), "provider")
	return plugin_mgr.Plugin{ID: "com.test.upload", Version: "0.1.0", State: plugin_mgr.StateInstalled, InstallMetadata: opts.Metadata}, nil
}
func TestLocalUploadKeepsOriginalMultipartContractWithoutMarketplace(t *testing.T) {
	for _, kind := range []string{"directory", "archive", "json_path"} {
		for _, enable := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/enable=%v", kind, enable), func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				source := t.TempDir()
				manifest := []byte("id: com.test.upload\nversion: 0.1.0\n")
				require.NoError(t, os.WriteFile(filepath.Join(source, "plugin.yaml"), manifest, 0o644))
				var body bytes.Buffer
				contentType := "application/json"
				if kind == "json_path" {
					require.NoError(t, json.NewEncoder(&body).Encode(map[string]any{"src_dir": source, "enable": enable, "force": true, "local_upload": true}))
				} else {
					writer := multipart.NewWriter(&body)
					require.NoError(t, writer.WriteField("enable", fmt.Sprint(enable)))
					require.NoError(t, writer.WriteField("force", "true"))
					require.NoError(t, writer.WriteField("metadata", `{"scope":"system","release_channel":"beta","notes":"local test"}`))
					if kind == "directory" {
						part, err := writer.CreateFormFile("files", "upload/plugin.yaml")
						require.NoError(t, err)
						_, err = part.Write(manifest)
						require.NoError(t, err)
						require.NoError(t, writer.WriteField("file_paths_json", `["upload/plugin.yaml"]`))
					} else {
						var archive bytes.Buffer
						gz := gzip.NewWriter(&archive)
						tr := tar.NewWriter(gz)
						require.NoError(t, tr.WriteHeader(&tar.Header{Name: "payload/plugin.yaml", Mode: 0o644, Size: int64(len(manifest))}))
						_, err := tr.Write(manifest)
						require.NoError(t, err)
						require.NoError(t, tr.Close())
						require.NoError(t, gz.Close())
						part, err := writer.CreateFormFile("file", "release.tar.gz")
						require.NoError(t, err)
						_, err = part.Write(archive.Bytes())
						require.NoError(t, err)
					}
					require.NoError(t, writer.Close())
					contentType = writer.FormDataContentType()
				}
				manager := &localUploadCompatibilityManager{t: t}
				mgrimpl.ResetGlobalForTest(manager)
				t.Cleanup(func() { mgrimpl.ResetGlobalForTest(nil) })
				engine := gin.New()
				engine.POST("/install/local", PluginInstallLocalHandler(&shared.Deps{}))
				request := httptest.NewRequest(http.MethodPost, "/install/local", &body)
				request.Header.Set("Content-Type", contentType)
				request = request.WithContext(reqctx.WithTenantUUID(reqctx.WithIsRoot(request.Context(), true), "00000000-0000-4000-8000-000000000001"))
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, request)
				if enable {
					// 凭证及 Topic 收尾依赖真实 DB。缺失时必须按原链路进入收尾并明确失败，不能假报启用成功。
					require.Equal(t, 500, response.Code, response.Body.String())
					require.Contains(t, response.Body.String(), "插件已安装")
				} else {
					require.Equal(t, 200, response.Code, response.Body.String())
				}
				require.Equal(t, 1, manager.calls)
				require.Equal(t, kind != "json_path", manager.options.LocalUpload)
				require.True(t, manager.options.Force)
				require.Equal(t, enable, manager.options.AutoEnable)
				require.Empty(t, manager.options.ExpectedRevision)
				require.False(t, manager.options.ConfirmLegacyProvider)
				require.Equal(t, "local", manager.options.Origin.Type)
				if kind != "json_path" {
					require.Equal(t, "beta", manager.options.Metadata.ReleaseChannel)
					_, err := os.Stat(manager.source)
					require.True(t, os.IsNotExist(err), "temporary uploads must be cleaned")
				} else {
					_, err := os.Stat(source)
					require.NoError(t, err, "JSON source directory is user-owned")
				}
			})
		}
	}
}
