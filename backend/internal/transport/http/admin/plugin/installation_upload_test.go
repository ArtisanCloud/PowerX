package plugin

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func taskUploadForm(t *testing.T, files map[string]string) *multipart.Form {
	t.Helper()
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	for path, body := range files {
		f, e := w.CreateFormFile("files", filepath.Base(path))
		require.NoError(t, e)
		_, e = f.Write([]byte(body))
		require.NoError(t, e)
		require.NoError(t, w.WriteField("file_paths", path))
	}
	require.NoError(t, w.Close())
	r := httptest.NewRequest("PUT", "/", &b)
	r.Header.Set("Content-Type", w.FormDataContentType())
	require.NoError(t, r.ParseMultipartForm(1<<20))
	t.Cleanup(func() { r.MultipartForm.RemoveAll() })
	return r.MultipartForm
}
func TestTaskUploadPreservesNestedDirectoryAndExecutable(t *testing.T) {
	form := taskUploadForm(t, map[string]string{"linux-amd64/plugin.yaml": "id: com.powerx.plugins.example\nversion: 1.0.0\n", "linux-amd64/backend/bin/plugin": "binary"})
	root, cleanup, e := prepareInstallationUpload(context.Background(), form)
	require.NoError(t, e)
	defer cleanup()
	require.Equal(t, "linux-amd64", filepath.Base(root))
	info, e := os.Stat(filepath.Join(root, "backend/bin/plugin"))
	require.NoError(t, e)
	require.NotZero(t, info.Mode()&0111)
}
func TestTaskUploadRejectsTraversalAndDuplicateEntries(t *testing.T) {
	for _, path := range []string{"../plugin.yaml", "/absolute/plugin.yaml", "parent/../../plugin.yaml", `parent\plugin.yaml`} {
		t.Run(path, func(t *testing.T) {
			form := taskUploadForm(t, map[string]string{path: "bad"})
			_, cleanup, e := prepareInstallationUpload(context.Background(), form)
			defer cleanup()
			require.Error(t, e)
		})
	}
	form := taskUploadForm(t, map[string]string{"plugin.yaml": "one", "other.yaml": "two"})
	form.Value["file_paths"] = []string{"plugin.yaml", "plugin.yaml"}
	_, cleanup, e := prepareInstallationUpload(context.Background(), form)
	defer cleanup()
	require.Error(t, e)
}
func TestTaskArchiveRejectsLinksTraversalAndExpansionBeforeWriting(t *testing.T) {
	for _, bad := range []string{"../escape", "link", "oversized"} {
		t.Run(bad, func(t *testing.T) {
			var packed bytes.Buffer
			gz := gzip.NewWriter(&packed)
			tw := tar.NewWriter(gz)
			hdr := &tar.Header{Name: "plugin.yaml", Mode: 0600, Size: 1, Typeflag: tar.TypeReg}
			if bad == "../escape" {
				hdr.Name = "../escape"
			}
			if bad == "link" {
				hdr.Typeflag = tar.TypeSymlink
				hdr.Linkname = "/etc/passwd"
				hdr.Size = 0
			}
			if bad == "oversized" {
				hdr.Size = 2 << 30
			}
			require.NoError(t, tw.WriteHeader(hdr))
			if hdr.Size == 1 {
				_, e := tw.Write([]byte("x"))
				require.NoError(t, e)
			}
			_ = tw.Close()
			_ = gz.Close()
			var b bytes.Buffer
			w := multipart.NewWriter(&b)
			f, e := w.CreateFormFile("file", "package.tgz")
			require.NoError(t, e)
			_, e = f.Write(packed.Bytes())
			require.NoError(t, e)
			require.NoError(t, w.Close())
			r := httptest.NewRequest("PUT", "/", &b)
			r.Header.Set("Content-Type", w.FormDataContentType())
			require.NoError(t, r.ParseMultipartForm(1<<20))
			defer r.MultipartForm.RemoveAll()
			_, cleanup, e := prepareInstallationUpload(context.Background(), r.MultipartForm)
			defer cleanup()
			require.Error(t, e)
		})
	}
}
func TestTaskUploadHonorsCancellation(t *testing.T) {
	form := taskUploadForm(t, map[string]string{"plugin.yaml": strings.Repeat("x", 1000)})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, cleanup, e := prepareInstallationUpload(ctx, form)
	defer cleanup()
	require.ErrorIs(t, e, context.Canceled)
}
