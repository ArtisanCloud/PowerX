package middleware

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	locadriver "github.com/ArtisanCloud/PowerX/internal/infra/media/driver/local"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
)

// RegisterLocalUploadEndpoint 注册本地驱动上传写入端点，配合预签名上传使用。
func RegisterLocalUploadEndpoint(r *gin.Engine, cfg *config.Config) {
	if r == nil || cfg == nil {
		return
	}
	localOpts := cfg.Storage.Local
	basePath := strings.TrimSpace(localOpts.BasePath)
	if basePath == "" {
		return
	}

	absBasePath, err := filepath.Abs(basePath)
	if err != nil {
		pxlog.Warn(nil, "resolve local media base path failed: "+err.Error())
		return
	}
	if err = os.MkdirAll(absBasePath, 0o755); err != nil {
		pxlog.Warn(nil, "init local media base path failed: "+err.Error())
		return
	}

	r.StaticFS("/media", gin.Dir(absBasePath, false))

	if !localOpts.EnableUploadEndpoint {
		return
	}
	secret := strings.TrimSpace(localOpts.UploadTokenSecret)
	if secret == "" {
		secret = strings.TrimSpace(cfg.Server.SecretKey)
	}
	maxSize := localOpts.MaxUploadSizeBytes
	if maxSize < 0 {
		maxSize = 0
	}
	handler, err := newLocalUploadHandler(absBasePath, secret, maxSize)
	if err != nil {
		pxlog.Warn(nil, "init local upload handler failed: "+err.Error())
		return
	}
	r.PUT("/media/*objectKey", handler.handle)
}

type localUploadHandler struct {
	basePath    string
	tokenSecret []byte
	maxSize     int64
}

func newLocalUploadHandler(basePath, secret string, maxSize int64) (*localUploadHandler, error) {
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
	return &localUploadHandler{
		basePath:    abs,
		tokenSecret: []byte(strings.TrimSpace(secret)),
		maxSize:     maxSize,
	}, nil
}

func (h *localUploadHandler) handle(c *gin.Context) {
	if c.Request.Method != http.MethodPut {
		c.AbortWithStatus(http.StatusMethodNotAllowed)
		return
	}
	objectKey := strings.Trim(c.Param("objectKey"), "/")
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
	if err = os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	file, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	defer file.Close()

	reader := io.Reader(c.Request.Body)
	limit := h.maxSize
	if limit > 0 {
		reader = io.LimitReader(c.Request.Body, limit+1)
	}
	written, err := io.Copy(file, reader)
	if err != nil {
		_ = os.Remove(dst)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	if h.maxSize > 0 {
		if written > h.maxSize || (c.Request.ContentLength == -1 && written > h.maxSize) {
			_ = file.Close()
			_ = os.Remove(dst)
			c.AbortWithStatus(http.StatusRequestEntityTooLarge)
			return
		}
		if c.Request.ContentLength > h.maxSize {
			_ = file.Close()
			_ = os.Remove(dst)
			c.AbortWithStatus(http.StatusRequestEntityTooLarge)
			return
		}
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
