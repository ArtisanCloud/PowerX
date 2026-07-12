package middleware

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	locadriver "github.com/ArtisanCloud/PowerX/internal/infra/media/driver/local"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

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
