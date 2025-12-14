package media

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// Handler 处理媒体资产 HTTP 请求。
type Handler struct {
	svc *mediasvc.MediaService
}

// NewHandler 构造 Handler。
func NewHandler(svc *mediasvc.MediaService) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) CreateAsset(c *gin.Context) {
	var req CreateAssetRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少有效租户上下文", err)
		return
	}
	operatorID := operatorIDFromRequest(c)
	size := int64(0)
	if req.SizeBytes != nil {
		size = *req.SizeBytes
	}

	asset, err := h.svc.CreateAsset(c.Request.Context(), mediasvc.CreateAssetInput{
		TenantUUID:   tenantUUID,
		OperatorID:   operatorID,
		Name:         req.Name,
		Description:  req.Description,
		Driver:       req.Driver,
		Bucket:       req.Bucket,
		BaseURL:      req.BaseURL,
		Folder:       req.Folder,
		StorageKey:   req.ObjectKey,
		SizeBytes:    size,
		MimeType:     req.MimeType,
		OwnerType:    req.OwnerSubjectType,
		OwnerID:      req.OwnerSubjectID,
		Tags:         req.Tags,
		UploadMethod: mediasvc.UploadMethod(req.UploadMethod),
		ExternalURL:  req.ExternalURL,
		Metadata:     req.Metadata,
	})
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "创建媒体资产失败", err)
		return
	}
	dto.ResponseSuccess(c, assetView(asset))
}

func (h *Handler) ListAssets(c *gin.Context) {
	var req ListAssetsRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少有效租户上下文", err)
		return
	}
	var drivers []string
	if driver := strings.TrimSpace(req.Driver); driver != "" {
		drivers = []string{driver}
	}

	assets, total, err := h.svc.ListAssets(c.Request.Context(), mediasvc.ListAssetsInput{
		TenantUUID:     tenantUUID,
		UUIDs:          req.UUIDs,
		Drivers:        drivers,
		OwnerType:      req.OwnerSubjectType,
		OwnerID:        req.OwnerSubjectID,
		Keyword:        req.Keyword,
		BusinessStatus: req.BusinessStatus,
		TagsAll:        req.Tags,
		IncludeDeleted: req.IncludeDeleted,
		OnlyDeleted:    req.OnlyDeleted,
		Page:           req.Page,
		PageSize:       req.PageSize,
	})
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "查询媒体资产失败", err)
		return
	}
	items := make([]assetResponse, 0, len(assets))
	for i := range assets {
		items = append(items, assetView(&assets[i]))
	}
	dto.ResponseList(c, items, &dto.PaginationResponse{Total: total, Page: req.Page, PageSize: req.PageSize})
}

func (h *Handler) GetAsset(c *gin.Context) {
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少有效租户上下文", err)
		return
	}
	uuid := c.Param("uuid")
	asset, err := h.svc.GetAsset(c.Request.Context(), tenantUUID, uuid, false)
	if err != nil {
		dto.ResponseError(c, http.StatusNotFound, "媒体资产不存在", err)
		return
	}
	dto.ResponseSuccess(c, assetView(asset))
}

func (h *Handler) UpdateAsset(c *gin.Context) {
	var req UpdateAssetRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少有效租户上下文", err)
		return
	}
	operatorID := operatorIDFromRequest(c)
	uuid := c.Param("uuid")

	asset, err := h.svc.UpdateAsset(c.Request.Context(), mediasvc.UpdateAssetInput{
		TenantUUID:     tenantUUID,
		UUID:           uuid,
		OperatorID:     operatorID,
		Name:           req.Name,
		Description:    req.Description,
		BusinessStatus: req.BusinessStatus,
		Tags:           req.Tags,
		Metadata:       req.Metadata,
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, mediasvc.ErrAssetNotFound) {
			status = http.StatusNotFound
		}
		dto.ResponseError(c, status, "更新媒体资产失败", err)
		return
	}
	dto.ResponseSuccess(c, assetView(asset))
}

func (h *Handler) DeleteAsset(c *gin.Context) {
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少有效租户上下文", err)
		return
	}
	operatorID := operatorIDFromRequest(c)
	uuid := c.Param("uuid")
	err = h.svc.DeleteAsset(c.Request.Context(), mediasvc.DeleteAssetInput{
		TenantUUID: tenantUUID,
		UUID:       uuid,
		OperatorID: operatorID,
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, mediasvc.ErrAssetNotFound) {
			status = http.StatusNotFound
		}
		dto.ResponseError(c, status, "删除媒体资产失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"deleted": true})
}

func (h *Handler) PresignAsset(c *gin.Context) {
	var req PresignRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少有效租户上下文", err)
		return
	}
	operatorID := operatorIDFromRequest(c)
	uuid := c.Param("uuid")

	ttlSeconds := req.ExpiresInSeconds
	if ttlSeconds <= 0 && req.ExpiresIn > 0 {
		ttlSeconds = req.ExpiresIn
	}
	ttl := time.Duration(ttlSeconds) * time.Second
	headers := http.Header{}
	for k, v := range req.Headers {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			continue
		}
		headers.Set(k, v)
	}
	if ct := strings.TrimSpace(req.ContentType); ct != "" {
		headers.Set("Content-Type", ct)
	}
	out, err := h.svc.PresignAsset(c.Request.Context(), mediasvc.PresignAssetInput{
		TenantUUID:  tenantUUID,
		UUID:        uuid,
		OperatorID:  operatorID,
		Action:      req.Action,
		Method:      req.Method,
		TTL:         ttl,
		Headers:     headers,
		ContentType: strings.TrimSpace(req.ContentType),
	})
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "生成预签名链接失败", err)
		return
	}
	responseHeaders := flattenHeaders(out.Headers)
	expiresIn := time.Until(out.ExpireAt)
	if expiresIn < 0 {
		expiresIn = 0
	}
	dto.ResponseSuccess(c, gin.H{
		"url":              out.URL,
		"method":           out.Method,
		"expiresInSeconds": int64(expiresIn / time.Second),
		"expiresAt":        out.ExpireAt.UTC().Format(time.RFC3339),
		"headers":          responseHeaders,
		"objectKey":        out.ObjectKey,
		"storageKey":       out.ObjectKey,
		"storage_key":      out.ObjectKey,
	})
}

func operatorIDFromRequest(c *gin.Context) *uint64 {
	value := strings.TrimSpace(c.GetHeader("X-Operator-ID"))
	if value == "" {
		return nil
	}
	id, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return nil
	}
	return &id
}

type assetResponse struct {
	UUID              string         `json:"uuid"`
	TenantUUID        string         `json:"tenant_uuid"`
	Name              string         `json:"name"`
	Description       string         `json:"description,omitempty"`
	Driver            string         `json:"driver"`
	Folder            string         `json:"folder,omitempty"`
	ObjectKey         string         `json:"objectKey"`
	ExternalURL       string         `json:"externalUrl,omitempty"`
	SizeBytes         *int64         `json:"sizeBytes,omitempty"`
	MimeType          string         `json:"mimeType,omitempty"`
	OwnerSubjectType  string         `json:"ownerSubjectType,omitempty"`
	OwnerSubjectID    string         `json:"ownerSubjectId,omitempty"`
	Tags              []string       `json:"tags,omitempty"`
	BusinessStatus    string         `json:"businessStatus"`
	DownloadURL       string         `json:"downloadUrl,omitempty"`
	DownloadExpiredAt *time.Time     `json:"downloadExpiredAt,omitempty"`
	CreatedAt         time.Time      `json:"createdAt"`
	UpdatedAt         time.Time      `json:"updatedAt"`
	Deleted           bool           `json:"deleted,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

func assetView(asset *mediasvc.Asset) assetResponse {
	if asset == nil {
		return assetResponse{}
	}
	resp := assetResponse{
		UUID:              asset.UUID,
		TenantUUID:        asset.TenantUUID,
		Name:              asset.Name,
		Description:       asset.Description,
		Driver:            asset.Driver,
		Folder:            asset.Folder,
		ObjectKey:         asset.StorageKey,
		ExternalURL:       asset.ExternalURL,
		MimeType:          asset.MimeType,
		OwnerSubjectType:  asset.OwnerType,
		OwnerSubjectID:    asset.OwnerID,
		Tags:              append([]string(nil), asset.Tags...),
		BusinessStatus:    asset.BusinessStatus,
		DownloadURL:       asset.DownloadURL,
		DownloadExpiredAt: asset.DownloadExpiry,
		CreatedAt:         asset.CreatedAt,
		UpdatedAt:         asset.UpdatedAt,
		Deleted:           asset.Deleted,
		Metadata:          cloneMetadataMap(asset.Metadata),
	}
	if asset.SizeBytes > 0 {
		size := asset.SizeBytes
		resp.SizeBytes = &size
	}
	return resp
}

func cloneMetadataMap(meta map[string]any) map[string]any {
	if len(meta) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(meta))
	for k, v := range meta {
		cloned[k] = v
	}
	return cloned
}

func flattenHeaders(headers http.Header) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	result := make(map[string]string, len(headers))
	for key, values := range headers {
		if len(values) == 0 {
			continue
		}
		result[key] = values[0]
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
