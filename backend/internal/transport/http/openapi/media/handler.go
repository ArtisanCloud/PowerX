package media

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// Handler exposes MediaService to tenant-facing open API callers.
type Handler struct {
	svc *mediasvc.MediaService
}

func NewHandler(svc *mediasvc.MediaService) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) CreateAsset(c *gin.Context) {
	var req CreateAssetRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		respondError(c, http.StatusUnauthorized, "missing tenant context", err)
		return
	}
	sizeBytes := int64(0)
	if req.SizeBytes != nil {
		sizeBytes = *req.SizeBytes
	}
	asset, err := h.svc.CreateAsset(c.Request.Context(), mediasvc.CreateAssetInput{
		TenantUUID:   tenantUUID,
		Name:         strings.TrimSpace(req.Name),
		Description:  req.Description,
		Driver:       req.Driver,
		Bucket:       req.Bucket,
		BaseURL:      req.BaseURL,
		Folder:       req.Folder,
		StorageKey:   req.ObjectKey,
		SizeBytes:    sizeBytes,
		MimeType:     req.MimeType,
		OwnerType:    req.OwnerSubjectType,
		OwnerID:      req.OwnerSubjectID,
		Tags:         req.Tags,
		UploadMethod: mediasvc.UploadMethod(req.UploadMethod),
		ExternalURL:  req.ExternalURL,
		Metadata:     metadataToAny(req.Metadata),
	})
	if err != nil {
		respondError(c, http.StatusBadRequest, "create media asset failed", err)
		return
	}
	respondCreated(c, assetView(asset))
}

func (h *Handler) ListAssets(c *gin.Context) {
	var req ListAssetsRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	req.Page = normalizePositive(req.Page, 1)
	req.PageSize = normalizePositive(req.PageSize, 20)
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		respondError(c, http.StatusUnauthorized, "missing tenant context", err)
		return
	}
	assets, total, err := h.svc.ListAssets(c.Request.Context(), mediasvc.ListAssetsInput{
		TenantUUID:     tenantUUID,
		Drivers:        oneOrZero(req.Driver),
		OwnerType:      req.OwnerSubjectType,
		OwnerID:        req.OwnerSubjectID,
		Keyword:        req.Keyword,
		TagsAll:        req.Tags,
		Page:           req.Page,
		PageSize:       req.PageSize,
		BusinessStatus: req.BusinessStatus,
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "list media assets failed", err)
		return
	}
	items := make([]assetViewResponse, 0, len(assets))
	for idx := range assets {
		items = append(items, assetView(&assets[idx]))
	}
	respondSuccess(c, gin.H{
		"items":    items,
		"total":    total,
		"page":     req.Page,
		"pageSize": req.PageSize,
	})
}

func (h *Handler) GetAsset(c *gin.Context) {
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		respondError(c, http.StatusUnauthorized, "missing tenant context", err)
		return
	}
	uuid := c.Param("uuid")
	asset, err := h.svc.GetAsset(c.Request.Context(), tenantUUID, uuid, false)
	if err != nil {
		respondError(c, http.StatusNotFound, "media asset not found", err)
		return
	}
	respondSuccess(c, assetView(asset))
}

func (h *Handler) UpdateAsset(c *gin.Context) {
	var req UpdateAssetRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		respondError(c, http.StatusUnauthorized, "missing tenant context", err)
		return
	}
	asset, err := h.svc.UpdateAsset(c.Request.Context(), mediasvc.UpdateAssetInput{
		TenantUUID:     tenantUUID,
		UUID:           c.Param("uuid"),
		Name:           req.Name,
		Description:    req.Description,
		BusinessStatus: req.BusinessStatus,
		Tags:           req.Tags,
		Metadata:       metadataToAny(req.Metadata),
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, mediasvc.ErrAssetNotFound) {
			status = http.StatusNotFound
		}
		respondError(c, status, "update media asset failed", err)
		return
	}
	respondSuccess(c, assetView(asset))
}

func (h *Handler) DeleteAsset(c *gin.Context) {
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		respondError(c, http.StatusUnauthorized, "missing tenant context", err)
		return
	}
	if err := h.svc.DeleteAsset(c.Request.Context(), mediasvc.DeleteAssetInput{
		TenantUUID: tenantUUID,
		UUID:       c.Param("uuid"),
	}); err != nil {
		if errors.Is(err, mediasvc.ErrAssetNotFound) {
			respondError(c, http.StatusNotFound, "media asset not found", err)
			return
		}
		respondError(c, http.StatusBadRequest, "delete media asset failed", err)
		return
	}
	respondSuccess(c, gin.H{"deleted": true})
}

func (h *Handler) PresignAsset(c *gin.Context) {
	var req PresignRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		respondError(c, http.StatusUnauthorized, "missing tenant context", err)
		return
	}
	if req.ExpiresInSeconds <= 0 {
		req.ExpiresInSeconds = 43200
	}
	out, err := h.svc.PresignAsset(c.Request.Context(), mediasvc.PresignAssetInput{
		TenantUUID: tenantUUID,
		UUID:       c.Param("uuid"),
		Action:     req.Action,
		Method:     req.Method,
		TTL:        time.Duration(req.ExpiresInSeconds) * time.Second,
	})
	if err != nil {
		respondError(c, http.StatusBadRequest, "generate presign url failed", err)
		return
	}
	respondSuccess(c, gin.H{
		"url":              out.URL,
		"method":           out.Method,
		"expiresInSeconds": req.ExpiresInSeconds,
		"headers":          flattenHeaders(out.Headers),
		"objectKey":        out.ObjectKey,
	})
}

func (h *Handler) CreateAssetVariant(c *gin.Context) {
	var req CreateVariantRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		respondError(c, http.StatusUnauthorized, "missing tenant context", err)
		return
	}
	sizeBytes := int64(0)
	if req.SizeBytes != nil {
		sizeBytes = *req.SizeBytes
	}
	variant, err := h.svc.CreateAssetVariant(c.Request.Context(), mediasvc.CreateAssetVariantInput{
		TenantUUID: tenantUUID,
		AssetUUID:  c.Param("uuid"),
		Variant:    c.Param("variant"),
		Name:       strings.TrimSpace(req.Name),
		Driver:     req.Driver,
		Bucket:     req.Bucket,
		BaseURL:    req.BaseURL,
		StorageKey: req.ObjectKey,
		SizeBytes:  sizeBytes,
		MimeType:   req.MimeType,
		Metadata:   metadataToAny(req.Metadata),
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, mediasvc.ErrAssetNotFound) {
			status = http.StatusNotFound
		}
		respondError(c, status, "create media asset variant failed", err)
		return
	}
	respondCreated(c, assetVariantView(variant))
}

func (h *Handler) PresignAssetVariant(c *gin.Context) {
	var req PresignRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		respondError(c, http.StatusUnauthorized, "missing tenant context", err)
		return
	}
	if req.ExpiresInSeconds <= 0 {
		req.ExpiresInSeconds = 43200
	}
	out, err := h.svc.PresignAssetVariant(c.Request.Context(), mediasvc.PresignAssetInput{
		TenantUUID: tenantUUID,
		UUID:       c.Param("uuid"),
		Variant:    c.Param("variant"),
		Action:     req.Action,
		Method:     req.Method,
		TTL:        time.Duration(req.ExpiresInSeconds) * time.Second,
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, mediasvc.ErrAssetNotFound) {
			status = http.StatusNotFound
		}
		respondError(c, status, "generate media asset variant presign url failed", err)
		return
	}
	respondSuccess(c, gin.H{
		"url":              out.URL,
		"method":           out.Method,
		"expiresInSeconds": req.ExpiresInSeconds,
		"headers":          flattenHeaders(out.Headers),
		"objectKey":        out.ObjectKey,
	})
}

func (h *Handler) StreamAssetResource(c *gin.Context) {
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		respondError(c, http.StatusUnauthorized, "missing tenant context", err)
		return
	}
	h.streamResource(c, tenantUUID)
}

func (h *Handler) StreamAssetVariantResource(c *gin.Context) {
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		respondError(c, http.StatusUnauthorized, "missing tenant context", err)
		return
	}
	asset, object, err := h.svc.OpenAssetVariantResource(c.Request.Context(), tenantUUID, c.Param("uuid"), c.Param("variant"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, mediasvc.ErrAssetNotFound) {
			status = http.StatusNotFound
		}
		respondError(c, status, "open media asset variant resource failed", err)
		return
	}
	if object == nil || object.Body == nil {
		respondError(c, http.StatusNotFound, "media asset variant object not found", nil)
		return
	}
	defer object.Body.Close()
	mimeType := strings.TrimSpace(asset.MimeType)
	if mimeType == "" {
		mimeType = strings.TrimSpace(object.ContentType)
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	filename := path.Base(strings.TrimSpace(asset.Name))
	if filename == "." || filename == "/" || filename == "" {
		filename = path.Base(strings.TrimSpace(asset.StorageKey))
	}
	if filename == "." || filename == "/" || filename == "" {
		filename = asset.Variant
	}
	disposition := sanitizeDisposition(c.DefaultQuery("disposition", "inline"))
	if object.Size > 0 {
		c.Header("Content-Length", strconv.FormatInt(object.Size, 10))
	}
	c.Header("Content-Type", mimeType)
	c.Header("Content-Disposition", fmt.Sprintf("%s; filename=%q", disposition, filename))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, object.Body)
}

// StreamAssetResourcePublic 提供无需租户上下文的公开访问。
func (h *Handler) StreamAssetResourcePublic(c *gin.Context) {
	uuid := c.Param("uuid")
	asset, object, err := h.svc.OpenAssetResource(c.Request.Context(), "", uuid)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, mediasvc.ErrAssetNotFound) {
			status = http.StatusNotFound
		}
		respondError(c, status, "open media asset resource failed", err)
		return
	}
	if asset == nil {
		respondError(c, http.StatusNotFound, "media asset not found", nil)
		return
	}
	// 默认仅允许 published 匿名访问；非 published 需带 token+exp（presign download 生成）
	if !h.svc.CanAccessPublicResource(asset, c.Query("exp"), c.Query("token")) {
		respondError(c, http.StatusNotFound, "media asset not found", nil)
		return
	}
	if asset.ExternalURL != "" {
		c.Redirect(http.StatusTemporaryRedirect, asset.ExternalURL)
		return
	}
	if object == nil || object.Body == nil {
		respondError(c, http.StatusNotFound, "media object not found", nil)
		return
	}
	defer object.Body.Close()

	mimeType := deriveMimeType(asset, object.ContentType)
	filename := ensureFileExtension(deriveFileName(asset), mimeType)
	disposition := sanitizeDisposition(c.DefaultQuery("disposition", "inline"))

	if object.Size > 0 {
		c.Header("Content-Length", strconv.FormatInt(object.Size, 10))
	}
	c.Header("Content-Type", mimeType)
	c.Header("Content-Disposition", fmt.Sprintf("%s; filename=%q", disposition, filename))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, object.Body); err != nil {
		c.Error(err)
	}
}

func (h *Handler) streamResource(c *gin.Context, tenantUUID string) {
	uuid := c.Param("uuid")
	asset, object, err := h.svc.OpenAssetResource(c.Request.Context(), tenantUUID, uuid)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, mediasvc.ErrAssetNotFound) {
			status = http.StatusNotFound
		}
		respondError(c, status, "open media asset resource failed", err)
		return
	}
	if asset == nil {
		respondError(c, http.StatusNotFound, "media asset not found", nil)
		return
	}
	if asset.ExternalURL != "" {
		c.Redirect(http.StatusTemporaryRedirect, asset.ExternalURL)
		return
	}
	if object == nil || object.Body == nil {
		respondError(c, http.StatusNotFound, "media object not found", nil)
		return
	}
	defer object.Body.Close()

	mimeType := deriveMimeType(asset, object.ContentType)
	filename := ensureFileExtension(deriveFileName(asset), mimeType)
	disposition := sanitizeDisposition(c.DefaultQuery("disposition", "inline"))

	if object.Size > 0 {
		c.Header("Content-Length", strconv.FormatInt(object.Size, 10))
	}
	c.Header("Content-Type", mimeType)
	c.Header("Content-Disposition", fmt.Sprintf("%s; filename=%q", disposition, filename))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, object.Body); err != nil {
		c.Error(err)
	}
}

func bindCreateRequest(c *gin.Context, req *CreateAssetRequest) error {
	return c.ShouldBind(req)
}

func oneOrZero(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return []string{trimmed}
}

func normalizePositive(value int, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func deriveMimeType(asset *mediasvc.Asset, fallback string) string {
	if asset != nil {
		if trimmed := strings.TrimSpace(asset.MimeType); trimmed != "" {
			return trimmed
		}
	}
	if trimmed := strings.TrimSpace(fallback); trimmed != "" {
		return trimmed
	}
	return "application/octet-stream"
}

func deriveFileName(asset *mediasvc.Asset) string {
	if asset == nil {
		return "media.bin"
	}
	if key := strings.TrimSpace(asset.StorageKey); key != "" {
		if base := path.Base(key); base != "" && base != "." && base != "/" {
			return base
		}
	}
	if name := strings.TrimSpace(asset.Name); name != "" {
		return name
	}
	if asset.UUID != "" {
		return asset.UUID
	}
	return "media.bin"
}

func sanitizeDisposition(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "attachment":
		return "attachment"
	default:
		return "inline"
	}
}

func ensureFileExtension(name, mimeType string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		trimmed = "media"
	}
	if strings.Contains(trimmed, ".") {
		return trimmed
	}
	if exts, _ := mime.ExtensionsByType(mimeType); len(exts) > 0 {
		return trimmed + exts[0]
	}
	switch strings.ToLower(mimeType) {
	case "image/jpeg":
		return trimmed + ".jpg"
	case "image/png":
		return trimmed + ".png"
	case "image/gif":
		return trimmed + ".gif"
	case "image/webp":
		return trimmed + ".webp"
	default:
		return trimmed
	}
}

func metadataToAny(src map[string]string) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		if strings.TrimSpace(k) == "" {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func flattenHeaders(headers http.Header) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for k, values := range headers {
		if len(values) == 0 {
			continue
		}
		out[k] = values[0]
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func respondSuccess(c *gin.Context, payload any) {
	if payload == nil {
		payload = gin.H{}
	}
	c.JSON(http.StatusOK, payload)
}

func respondCreated(c *gin.Context, payload any) {
	if payload == nil {
		payload = gin.H{}
	}
	c.JSON(http.StatusCreated, payload)
}

func respondError(c *gin.Context, status int, msg string, err error) {
	if status <= 0 {
		status = http.StatusInternalServerError
	}
	traceID := reqctx.GetTraceID(c.Request.Context())
	resp := gin.H{
		"code":    status,
		"message": msg,
	}
	if traceID != "" {
		resp["traceId"] = traceID
	}
	if err != nil {
		resp["error"] = err.Error()
	}
	c.JSON(status, resp)
}
