package middleware

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	locadriver "github.com/ArtisanCloud/PowerX/internal/infra/media/driver/local"
	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestLocalUploadAdmissionRejectsBeforeReadingAndPreservesExistingObject(t *testing.T) {
	dir := t.TempDir()
	c, err := budget.New(budget.Config{Directory: filepath.Join(t.TempDir(), "state"), FilesystemPath: dir, HostID: "host", FilesystemID: "disk", InstanceID: "dev", MinFreeBytes: 1 << 50, CriticalFreeBytes: 1})
	require.NoError(t, err)
	budget.SetDefault(c)
	t.Cleanup(func() { budget.SetDefault(nil) })
	cfg := &config.Config{}
	cfg.Storage.Local.BasePath, cfg.Storage.Local.UploadTokenSecret = dir, "acceptance-secret"
	router := gin.New()
	RegisterLocalUploadEndpoint(router.Group("/api/v1"), cfg, nil)
	object := filepath.Join(dir, "asset", "origin")
	require.NoError(t, os.MkdirAll(filepath.Dir(object), 0700))
	require.NoError(t, os.WriteFile(object, []byte("existing business object"), 0600))
	req := httptest.NewRequest(http.MethodPut, "/api/v1/media/assets/asset", nil)
	req.Body, req.ContentLength = unreadableUploadBody{t}, -1
	signLocalUpload(req, "acceptance-secret", "asset/origin")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusInsufficientStorage, rec.Code)
	raw, err := os.ReadFile(object)
	require.NoError(t, err)
	require.Equal(t, "existing business object", string(raw))
	state, err := c.Status(context.Background())
	require.NoError(t, err)
	require.Empty(t, state.Active)
}

type unreadableUploadBody struct{ t *testing.T }

func (r unreadableUploadBody) Read([]byte) (int, error) {
	r.t.Error("upload body was read despite rejected admission")
	return 0, context.Canceled
}
func (unreadableUploadBody) Close() error { return nil }

func signLocalUpload(req *http.Request, secret, key string) {
	expires := strconv.FormatInt(time.Now().Add(time.Minute).Unix(), 10)
	req.Header.Set(locadriver.HeaderUploadExpires, expires)
	req.Header.Set(locadriver.HeaderUploadToken, locadriver.GenerateUploadToken([]byte(secret), key, expires))
}

func TestLocalUploadStreamsMultipartAndFailedChunkedReplacementPreservesObject(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{}
	cfg.Storage.Local.BasePath, cfg.Storage.Local.UploadTokenSecret, cfg.Storage.Local.MaxUploadSizeBytes = dir, "acceptance-secret", 1024
	router := gin.New()
	RegisterLocalUploadEndpoint(router.Group("/api/v1"), cfg, nil)
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile(uploadFormField, "example.txt")
	require.NoError(t, err)
	_, err = part.Write([]byte("existing business object"))
	require.NoError(t, err)
	require.NoError(t, form.Close())
	req := httptest.NewRequest(http.MethodPut, "/api/v1/media/assets/asset", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	signLocalUpload(req, "acceptance-secret", "asset/origin")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)
	if req.MultipartForm != nil {
		require.Empty(t, req.MultipartForm.File, "multipart upload must stream instead of parsing/spooling files")
	}
	object := filepath.Join(dir, "asset", "origin")
	req = httptest.NewRequest(http.MethodPut, "/api/v1/media/assets/asset", strings.NewReader(strings.Repeat("x", 2048)))
	req.ContentLength = -1
	signLocalUpload(req, "acceptance-secret", "asset/origin")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	raw, err := os.ReadFile(object)
	require.NoError(t, err)
	require.Equal(t, "existing business object", string(raw))
	staged, err := filepath.Glob(filepath.Join(dir, "asset", ".media-upload-*"))
	require.NoError(t, err)
	require.Empty(t, staged)
}

func TestRegisterLocalUploadEndpoint_WritesVariantObject(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		secret    = "local-upload-test-secret"
		assetUUID = "6176f24f-91e2-4b9b-a274-f8ce734eaf51"
		variant   = "preview"
	)

	cfg := &config.Config{}
	cfg.Storage.Local.BasePath = t.TempDir()
	cfg.Storage.Local.UploadTokenSecret = secret
	cfg.Storage.Local.MaxUploadSizeBytes = 1024

	router := gin.New()
	group := router.Group("/api/v1")
	RegisterLocalUploadEndpoint(group, cfg, nil)

	expires := strconv.FormatInt(time.Now().Add(time.Minute).Unix(), 10)
	objectKey := assetUUID + "/" + variant
	req := httptest.NewRequest(http.MethodPut, "/api/v1/media/assets/"+assetUUID+"/variants/"+variant, strings.NewReader("preview"))
	req.Header.Set(locadriver.HeaderUploadExpires, expires)
	req.Header.Set(locadriver.HeaderUploadToken, locadriver.GenerateUploadToken([]byte(secret), objectKey, expires))
	req.Header.Set("Content-Type", "image/jpeg")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.FileExists(t, filepath.Join(cfg.Storage.Local.BasePath, assetUUID, variant))
}

func TestRegisterLocalUploadEndpoint_WritesOriginUnderAssetDirectory(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		secret    = "local-upload-test-secret"
		assetUUID = "6176f24f-91e2-4b9b-a274-f8ce734eaf51"
	)

	cfg := &config.Config{}
	cfg.Storage.Local.BasePath = t.TempDir()
	cfg.Storage.Local.UploadTokenSecret = secret
	cfg.Storage.Local.MaxUploadSizeBytes = 1024

	router := gin.New()
	group := router.Group("/api/v1")
	RegisterLocalUploadEndpoint(group, cfg, nil)

	expires := strconv.FormatInt(time.Now().Add(time.Minute).Unix(), 10)
	objectKey := assetUUID + "/origin"
	req := httptest.NewRequest(http.MethodPut, "/api/v1/media/assets/"+assetUUID, strings.NewReader("origin"))
	req.Header.Set(locadriver.HeaderUploadExpires, expires)
	req.Header.Set(locadriver.HeaderUploadToken, locadriver.GenerateUploadToken([]byte(secret), objectKey, expires))
	req.Header.Set("Content-Type", "image/png")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.FileExists(t, filepath.Join(cfg.Storage.Local.BasePath, assetUUID, "origin"))
}

func TestRegisterLocalUploadEndpoint_SkipsRegistrationWithoutUploadSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	cfg.Storage.Local.BasePath = t.TempDir()

	router := gin.New()
	group := router.Group("/api/v1")
	RegisterLocalUploadEndpoint(group, cfg, nil)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/media/assets/6176f24f-91e2-4b9b-a274-f8ce734eaf51", strings.NewReader("origin"))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}
