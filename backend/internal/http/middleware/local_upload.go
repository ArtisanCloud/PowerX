package middleware

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
	locadriver "github.com/ArtisanCloud/PowerX/internal/infra/media/driver/local"
	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	"github.com/ArtisanCloud/PowerX/internal/service/media"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
)

// RegisterLocalUploadEndpoint 注册本地驱动上传写入端点，配合预签名上传使用。
func RegisterLocalUploadEndpoint(group *gin.RouterGroup, cfg *config.Config, mediaSvc *media.MediaService) {
	if group == nil || cfg == nil {
		return
	}
	localOpts := cfg.Storage.Local
	basePath := strings.TrimSpace(localOpts.BasePath)
	if basePath == "" {
		return
	}

	absBasePath, err := filepath.Abs(basePath)
	if err != nil {
		pxlog.Warn(pxlog.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "resolve local media base path failed: "+err.Error())
		return
	}
	if err = os.MkdirAll(absBasePath, 0o755); err != nil {
		pxlog.Warn(pxlog.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "init local media base path failed: "+err.Error())
		return
	}

	secret := strings.TrimSpace(localOpts.UploadTokenSecret)
	if secret == "" {
		pxlog.Warn(pxlog.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "storage.local.upload_token_secret 未配置，本地上传端点不会注册")
		return
	}
	maxSize := localOpts.MaxUploadSizeBytes
	if maxSize < 0 {
		maxSize = 0
	}
	handler, err := newLocalUploadHandler(absBasePath, secret, maxSize, mediaSvc, "local")
	if err != nil {
		pxlog.Warn(pxlog.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "init local upload handler failed: "+err.Error())
		return
	}
	group.PUT("/media/assets/:uuid", handler.handle)
	group.PUT("/media/assets/:uuid/variants/:variant", handler.handle)
}

type localUploadHandler struct {
	basePath    string
	tokenSecret []byte
	maxSize     int64
	mediaSvc    *media.MediaService
	driverName  string
	storage     *locadriver.Driver
}

const uploadFormField = "upload-file"

func newLocalUploadHandler(basePath, secret string, maxSize int64, svc *media.MediaService, driverName string) (*localUploadHandler, error) {
	if strings.TrimSpace(basePath) == "" {
		return nil, errors.New("base path required")
	}
	abs, err := filepath.Abs(basePath)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	if maxSize < 0 {
		maxSize = 0
	}
	if strings.TrimSpace(driverName) == "" {
		driverName = "local"
	}
	storage, err := locadriver.New(locadriver.Options{BasePath: abs, Name: driverName})
	if err != nil {
		return nil, err
	}
	return &localUploadHandler{
		basePath:    abs,
		tokenSecret: []byte(strings.TrimSpace(secret)),
		maxSize:     maxSize,
		mediaSvc:    svc,
		driverName:  driverName,
		storage:     storage,
	}, nil
}

func (h *localUploadHandler) handle(c *gin.Context) {
	if c.Request.Method != http.MethodPut {
		c.AbortWithStatus(http.StatusMethodNotAllowed)
		return
	}
	objectKey := strings.TrimSpace(c.Param("uuid"))
	if variant := strings.TrimSpace(c.Param("variant")); variant != "" {
		objectKey = strings.TrimRight(objectKey, "/") + "/" + strings.TrimLeft(variant, "/")
	} else if objectKey != "" {
		objectKey = strings.TrimRight(objectKey, "/") + "/origin"
	}
	if objectKey == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "object key required"})
		return
	}
	expiresHeader := strings.TrimSpace(c.GetHeader(locadriver.HeaderUploadExpires))
	tokenHeader := strings.TrimSpace(c.GetHeader(locadriver.HeaderUploadToken))
	if len(h.tokenSecret) > 0 {
		if expiresHeader == "" || tokenHeader == "" {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		expUnix, err := strconv.ParseInt(expiresHeader, 10, 64)
		if err != nil {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		if time.Unix(expUnix, 0).Before(time.Now().Add(-5 * time.Second)) {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		if !locadriver.VerifyUploadToken(h.tokenSecret, objectKey, expiresHeader, tokenHeader) {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
	}
	if h.maxSize > 0 {
		if cl := c.Request.ContentLength; cl > h.maxSize {
			c.AbortWithStatus(http.StatusRequestEntityTooLarge)
			return
		}
	}
	dst, err := h.resolvePath(objectKey)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid object key"})
		return
	}
	peak := c.Request.ContentLength
	if peak <= 0 {
		peak = 1 << 20
	}
	ctx, release, err := budget.BeginPath(c.Request.Context(), h.basePath, "media_upload", peak)
	if err != nil {
		h.abortUploadError(c, "admission_failed", err)
		return
	}
	defer release()

	var mimeType string
	var body io.Reader = c.Request.Body
	size := c.Request.ContentLength
	if size < 0 {
		size = 0
	}
	contentType := strings.ToLower(strings.TrimSpace(c.GetHeader("Content-Type")))
	if strings.Contains(contentType, "multipart/form-data") {
		// Stream the part directly; ParseMultipartForm may spool outside the budget.
		reader, formErr := c.Request.MultipartReader()
		if formErr != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "upload-file field required"})
			return
		}
		found := false
		for i := 0; i < 16; i++ {
			part, partErr := reader.NextPart()
			if partErr != nil {
				break
			}
			if part.FormName() != uploadFormField {
				_ = part.Close()
				continue
			}
			defer part.Close()
			body, size = part, 0
			mimeType = strings.TrimSpace(part.Header.Get("Content-Type"))
			found = true
			break
		}
		if !found {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "upload-file field required"})
			return
		}
	}
	if h.maxSize > 0 {
		body = http.MaxBytesReader(c.Writer, io.NopCloser(body), h.maxSize)
	}
	_, err = h.storage.Put(ctx, driver.PutObjectInput{ObjectKey: objectKey, Body: body, Size: size, Overwrite: true})
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.AbortWithStatus(http.StatusRequestEntityTooLarge)
			return
		}
		h.abortUploadError(c, "copy_failed", err)
		return
	}
	file, err := os.Open(dst)
	if err != nil {
		h.abortUploadError(c, "open_file_failed", err)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		h.abortUploadError(c, "stat_failed", err)
		return
	}
	if strings.TrimSpace(mimeType) == "" {
		mimeType = detectFileContentType(file)
	}

	if err := h.syncUploadedAsset(ctx, objectKey, info.Size(), mimeType); err != nil {
		h.abortUploadError(c, "sync_metadata_failed", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *localUploadHandler) resolvePath(objectKey string) (string, error) {
	sanitized := filepath.Clean(strings.TrimLeft(objectKey, "/"))
	if sanitized == "." || sanitized == ".." || sanitized == "" {
		return "", errors.New("invalid object key")
	}
	if strings.HasPrefix(sanitized, "../") || strings.HasPrefix(sanitized, "..\\") {
		return "", errors.New("invalid object key")
	}
	full := filepath.Join(h.basePath, sanitized)
	rel, err := filepath.Rel(h.basePath, full)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(rel, "..") {
		return "", errors.New("invalid object key")
	}
	return full, nil
}

func detectFileContentType(f *os.File) string {
	if f == nil {
		return ""
	}
	current, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return ""
	}
	defer f.Seek(current, io.SeekStart)
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return ""
	}
	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return ""
	}
	if n <= 0 {
		return ""
	}
	return http.DetectContentType(buf[:n])
}

func (h *localUploadHandler) syncUploadedAsset(ctx context.Context, objectKey string, size int64, mimeType string) error {
	if h.mediaSvc == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := h.mediaSvc.SyncUploadedFileMetadata(ctx, h.driverName, objectKey, size, mimeType); err != nil {
		pxlog.Warn(ctx, "sync uploaded asset metadata failed: "+err.Error())
		return err
	}
	return nil
}

func (h *localUploadHandler) abortUploadError(c *gin.Context, reason string, err error) {
	ctx := c.Request.Context()
	pxlog.Error(pxlog.WithLogFields(ctx, map[string]interface{}{
		"module":      "media",
		"component":   "local_upload",
		"reason":      reason,
		"driver":      h.driverName,
		"request_uri": c.Request.RequestURI,
	}), "local media upload failed: "+err.Error())
	status := http.StatusInternalServerError
	if errors.Is(err, budget.ErrInsufficient) {
		status = http.StatusInsufficientStorage
	} else if errors.Is(err, budget.ErrBusy) || errors.Is(err, budget.ErrUnavailable) {
		status = http.StatusServiceUnavailable
	}
	c.AbortWithStatusJSON(status, gin.H{
		"error":  "local media upload failed",
		"reason": reason,
	})
}
