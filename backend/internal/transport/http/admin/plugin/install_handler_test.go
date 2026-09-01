package plugin

import (
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
)

func TestUploadedDirRelPathsPrefersLegacyFilePaths(t *testing.T) {
	form := &multipart.Form{
		Value: map[string][]string{
			"file_paths":      {"legacy/plugin.yaml"},
			"file_paths_json": {`["json/plugin.yaml"]`},
		},
	}

	got := uploadedDirRelPaths(form)
	if len(got) != 1 || got[0] != "legacy/plugin.yaml" {
		t.Fatalf("uploadedDirRelPaths() = %#v, want legacy file_paths", got)
	}
}

func TestCoalesceInstallMetadataRejectsDeprecatedEnvironment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	_, err := coalesceInstallMetadata(ctx, plugin_mgr.InstallMetadata{
		DeprecatedEnvironment: json.RawMessage(`"prod"`),
	})
	if err == nil || err.Error() != "PLUGIN_INSTALL_METADATA_ENVIRONMENT_DEPRECATED: use metadata.release_channel" {
		t.Fatalf("coalesceInstallMetadata() error = %v", err)
	}
}

func TestCoalesceInstallMetadataKeepsReleaseChannelSeparate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	got, err := coalesceInstallMetadata(ctx, plugin_mgr.InstallMetadata{ReleaseChannel: " beta "})
	if err != nil {
		t.Fatalf("coalesceInstallMetadata() error = %v", err)
	}
	if got.ReleaseChannel != "beta" {
		t.Fatalf("release_channel = %q, want beta", got.ReleaseChannel)
	}
}

func TestUploadedDirRelPathsReadsJSONList(t *testing.T) {
	form := &multipart.Form{
		Value: map[string][]string{
			"file_paths_json": {`["mac/plugin.yaml","mac/web-admin/app.vue"]`},
		},
	}

	got := uploadedDirRelPaths(form)
	if len(got) != 2 || got[0] != "mac/plugin.yaml" || got[1] != "mac/web-admin/app.vue" {
		t.Fatalf("uploadedDirRelPaths() = %#v, want JSON file paths", got)
	}
}

func TestUploadedDirRelPathsInvalidJSONReturnsNil(t *testing.T) {
	form := &multipart.Form{
		Value: map[string][]string{
			"file_paths_json": {"not-json"},
		},
	}

	if got := uploadedDirRelPaths(form); got != nil {
		t.Fatalf("uploadedDirRelPaths() = %#v, want nil", got)
	}
}
