package media

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// hostContractHandler is the UUID-only delegated Media Host Contract. It does
// not expose object_key, bucket, or long-lived storage credentials.
type hostContractHandler struct {
	svc    *mediasvc.MediaService
	access *mediasvc.HostContractAccess
}

type hostCreateAssetRequest struct {
	Name       string `json:"name" binding:"required"`
	MimeType   string `json:"mime_type" binding:"required"`
	SizeBytes  int64  `json:"size_bytes" binding:"gt=0"`
	Checksum   string `json:"checksum" binding:"required"`
	TenantUUID string `json:"tenant_uuid"`
}
type hostUpdateAssetRequest struct {
	Name       *string  `json:"name"`
	Tags       []string `json:"tags"`
	TenantUUID string   `json:"tenant_uuid"`
}
type hostCompleteUploadRequest struct {
	Checksum   string `json:"checksum" binding:"required"`
	TenantUUID string `json:"tenant_uuid"`
}
type hostCreateVariantRequest struct {
	Checksum    string `json:"checksum" binding:"required,len=64,hexadecimal"`
	VariantType string `json:"variant_type" binding:"required,oneof=preview thumbnail"`
	Name        string `json:"name"`
	MimeType    string `json:"mime_type" binding:"required"`
	SizeBytes   int64  `json:"size_bytes" binding:"gt=0"`
	TenantUUID  string `json:"tenant_uuid"`
}
type hostPresignRequest struct {
	ExpiresInSeconds int64  `json:"expires_in_seconds" binding:"omitempty,min=60,max=3600"`
	TenantUUID       string `json:"tenant_uuid"`
}
type hostAssetResponse struct {
	AssetUUID        string     `json:"asset_uuid"`
	Name             string     `json:"name"`
	SizeBytes        int64      `json:"size_bytes"`
	MimeType         string     `json:"mime_type"`
	OwnerSubjectUUID string     `json:"owner_subject_uuid,omitempty"`
	Status           string     `json:"status"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	DeletedAt        *time.Time `json:"deleted_at,omitempty"`
}

func registerHostContract(protected *gin.RouterGroup, deps *shared.Deps) {
	if protected == nil || deps == nil || deps.MediaSvc == nil || deps.DB == nil {
		return
	}
	h := &hostContractHandler{svc: deps.MediaSvc, access: mediasvc.NewHostContractAccess(deps.DB)}
	group := protected.Group("/tenant/media/assets")
	group.GET("", h.list)
	group.GET("/:asset_uuid", h.get)
	group.POST("", h.create)
	group.PATCH("/:asset_uuid", h.update)
	group.DELETE("/:asset_uuid", h.delete)
	group.POST("/:asset_uuid/variants", h.createVariant)
	group.GET("/variants/:variant_uuid", h.getVariant)
	group.POST("/:asset_uuid/variants/:variant_uuid/presign-upload", h.variantPresignUpload)
	group.POST("/:asset_uuid/variants/:variant_uuid/complete-upload", h.variantComplete)
	group.POST("/:asset_uuid/variants/:variant_uuid/presign-download", h.variantPresignDownload)
	group.POST("/:asset_uuid/presign-upload", h.presignUpload)
	group.POST("/:asset_uuid/complete-upload", h.completeUpload)
	group.POST("/:asset_uuid/presign-download", h.presignDownload)
}

func (h *hostContractHandler) createVariant(c *gin.Context) {
	var req hostCreateVariantRequest
	if err := decodeVariantBody(c, &req, "variant_type", "name", "mime_type", "size_bytes", "checksum"); err != nil {
		dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(err))
		return
	}
	if strings.TrimSpace(req.TenantUUID) != "" || !hostRejectTenantOverride(c) {
		if strings.TrimSpace(req.TenantUUID) != "" {
			dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(errors.New("tenant_uuid must not be supplied")))
		}
		return
	}
	assetUUID, err := hostUUID(c.Param("asset_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(err))
		return
	}
	tenantUUID, err := h.access.AuthorizeVariantsManage(c.Request.Context(), hostAPIKeyHash(c))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	variant, err := h.svc.CreateAssetVariant(c.Request.Context(), mediasvc.CreateAssetVariantInput{TenantUUID: tenantUUID, AssetUUID: assetUUID, Variant: req.VariantType, Name: req.Name, MimeType: req.MimeType, SizeBytes: req.SizeBytes, Checksum: strings.ToLower(req.Checksum)})
	if err != nil {
		dto.RespondErrorFrom(c, hostMediaError(err))
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, gin.H{"variant_uuid": variant.UUID, "asset_uuid": variant.AssetUUID, "name": variant.Name, "mime_type": variant.MimeType, "size_bytes": variant.SizeBytes, "status": variant.UploadState, "completed_at": variant.CompletedAt})
}

func (h *hostContractHandler) getVariant(c *gin.Context) {
	if !hostRejectTenantOverride(c) {
		return
	}
	variantUUID, err := hostUUID(c.Param("variant_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(err))
		return
	}
	tenantUUID, err := h.access.AuthorizeRead(c.Request.Context(), hostAPIKeyHash(c))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	variant, err := h.svc.GetAssetVariantByUUID(c.Request.Context(), tenantUUID, variantUUID)
	if err != nil {
		dto.RespondErrorFrom(c, hostMediaError(err))
		return
	}
	dto.ResponseSuccess(c, gin.H{"variant_uuid": variant.UUID, "asset_uuid": variant.AssetUUID, "name": variant.Name, "mime_type": variant.MimeType, "size_bytes": variant.SizeBytes, "status": variant.UploadState, "completed_at": variant.CompletedAt})
}

func (h *hostContractHandler) list(c *gin.Context) {
	if !hostRejectTenantOverride(c) {
		return
	}
	tenantUUID, err := h.access.AuthorizeRead(c.Request.Context(), hostAPIKeyHash(c))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	page, pageSize := 1, 20
	if raw := c.Query("page"); raw != "" {
		page, _ = strconv.Atoi(raw)
	}
	if raw := c.Query("page_size"); raw != "" {
		pageSize, _ = strconv.Atoi(raw)
	}
	if page < 1 || pageSize < 1 || pageSize > 100 {
		dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(errors.New("invalid pagination")))
		return
	}
	assets, total, err := h.svc.ListAssets(c.Request.Context(), mediasvc.ListAssetsInput{TenantUUID: tenantUUID, Page: page, PageSize: pageSize})
	if err != nil {
		dto.RespondErrorFrom(c, mediasvc.MediaUpstreamDependencyError(err))
		return
	}
	items := make([]hostAssetResponse, 0, len(assets))
	for i := range assets {
		items = append(items, hostAssetView(&assets[i]))
	}
	dto.ResponseSuccess(c, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize})
}

func (h *hostContractHandler) get(c *gin.Context) {
	if !hostRejectTenantOverride(c) {
		return
	}
	assetUUID, err := hostUUID(c.Param("asset_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(err))
		return
	}
	tenantUUID, err := h.access.AuthorizeRead(c.Request.Context(), hostAPIKeyHash(c))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	asset, err := h.svc.GetAsset(c.Request.Context(), tenantUUID, assetUUID, false)
	if err != nil {
		dto.RespondErrorFrom(c, hostMediaError(err))
		return
	}
	dto.ResponseSuccess(c, hostAssetView(asset))
}

func (h *hostContractHandler) create(c *gin.Context) {
	var req hostCreateAssetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(err))
		return
	}
	if strings.TrimSpace(req.TenantUUID) != "" || !hostRejectTenantOverride(c) {
		if strings.TrimSpace(req.TenantUUID) != "" {
			dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(errors.New("tenant_uuid must not be supplied")))
		}
		return
	}
	tenantUUID, err := h.access.AuthorizeManage(c.Request.Context(), hostAPIKeyHash(c))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	asset, err := h.svc.CreateAsset(c.Request.Context(), mediasvc.CreateAssetInput{TenantUUID: tenantUUID, Name: req.Name, SizeBytes: req.SizeBytes, MimeType: req.MimeType, ContentSHA256: req.Checksum, UploadMethod: mediasvc.UploadMethodPresign})
	if err != nil {
		dto.RespondErrorFrom(c, hostMediaError(err))
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, hostAssetView(asset))
}

func (h *hostContractHandler) update(c *gin.Context) {
	var req hostUpdateAssetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(err))
		return
	}
	if strings.TrimSpace(req.TenantUUID) != "" || !hostRejectTenantOverride(c) {
		if strings.TrimSpace(req.TenantUUID) != "" {
			dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(errors.New("tenant_uuid must not be supplied")))
		}
		return
	}
	assetUUID, err := hostUUID(c.Param("asset_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(err))
		return
	}
	tenantUUID, err := h.access.AuthorizeManage(c.Request.Context(), hostAPIKeyHash(c))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	asset, err := h.svc.UpdateAsset(c.Request.Context(), mediasvc.UpdateAssetInput{TenantUUID: tenantUUID, UUID: assetUUID, Name: req.Name, Tags: req.Tags})
	if err != nil {
		dto.RespondErrorFrom(c, hostMediaError(err))
		return
	}
	dto.ResponseSuccess(c, hostAssetView(asset))
}

func (h *hostContractHandler) delete(c *gin.Context) {
	if !hostRejectTenantOverride(c) {
		return
	}
	assetUUID, err := hostUUID(c.Param("asset_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(err))
		return
	}
	tenantUUID, err := h.access.AuthorizeManage(c.Request.Context(), hostAPIKeyHash(c))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	if err = h.svc.DeleteAsset(c.Request.Context(), mediasvc.DeleteAssetInput{TenantUUID: tenantUUID, UUID: assetUUID}); err != nil {
		dto.RespondErrorFrom(c, hostMediaError(err))
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *hostContractHandler) presignUpload(c *gin.Context) {
	assetUUID, err := hostUUID(c.Param("asset_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(err))
		return
	}
	h.presign(c, assetUUID, "upload")
}

func (h *hostContractHandler) presignDownload(c *gin.Context) {
	assetUUID, err := hostUUID(c.Param("asset_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(err))
		return
	}
	h.presign(c, assetUUID, "download")
}

func (h *hostContractHandler) completeUpload(c *gin.Context) {
	assetUUID, err := hostUUID(c.Param("asset_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(err))
		return
	}
	h.complete(c, assetUUID)
}

func (h *hostContractHandler) presign(c *gin.Context, assetUUID, action string) {
	var req hostPresignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(err))
		return
	}
	if strings.TrimSpace(req.TenantUUID) != "" || !hostRejectTenantOverride(c) {
		if strings.TrimSpace(req.TenantUUID) != "" {
			dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(errors.New("tenant_uuid must not be supplied")))
		}
		return
	}
	tenantUUID, err := h.access.AuthorizeTransfer(c.Request.Context(), hostAPIKeyHash(c))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	asset, err := h.svc.GetAsset(c.Request.Context(), tenantUUID, assetUUID, false)
	if err != nil {
		dto.RespondErrorFrom(c, hostMediaError(err))
		return
	}
	if action == "upload" && asset.UploadState != "pending_upload" {
		dto.RespondErrorFrom(c, mediasvc.MediaAssetStateInvalidError(errors.New("asset is not pending upload")))
		return
	}
	if action == "download" && asset.UploadState != "ready" {
		dto.RespondErrorFrom(c, mediasvc.MediaAssetStateInvalidError(errors.New("asset is not ready")))
		return
	}
	ttl := req.ExpiresInSeconds
	if ttl == 0 {
		ttl = 900
	}
	ticket, err := h.svc.IssueHostTransferTicket(c.Request.Context(), tenantUUID, assetUUID, action, time.Duration(ttl)*time.Second)
	if err != nil {
		dto.RespondErrorFrom(c, hostMediaError(err))
		return
	}
	dto.ResponseSuccess(c, gin.H{"url": "/api/v1/media/transfers/" + ticket.AssetUUID + "/" + ticket.Action + "?exp=" + strconv.FormatInt(ticket.ExpiresAt.Unix(), 10) + "&version=" + strconv.FormatUint(ticket.Version, 10) + "&ticket=" + ticket.Token, "method": map[bool]string{true: http.MethodPut, false: http.MethodGet}[action == "upload"], "expires_at": ticket.ExpiresAt, "headers": map[string]string{}})
}

func (h *hostContractHandler) complete(c *gin.Context, assetUUID string) {
	var req hostCompleteUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(err))
		return
	}
	if strings.TrimSpace(req.TenantUUID) != "" || !hostRejectTenantOverride(c) {
		if strings.TrimSpace(req.TenantUUID) != "" {
			dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(errors.New("tenant_uuid must not be supplied")))
		}
		return
	}
	tenantUUID, err := h.access.AuthorizeTransfer(c.Request.Context(), hostAPIKeyHash(c))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	asset, err := h.svc.CompleteAssetUpload(c.Request.Context(), mediasvc.CompleteAssetUploadInput{TenantUUID: tenantUUID, UUID: assetUUID, Checksum: req.Checksum})
	if err != nil {
		dto.RespondErrorFrom(c, hostMediaError(err))
		return
	}
	dto.ResponseSuccess(c, hostAssetView(asset))
}

func hostAssetView(asset *mediasvc.Asset) hostAssetResponse {
	return hostAssetResponse{AssetUUID: asset.UUID, Name: asset.Name, SizeBytes: asset.SizeBytes, MimeType: asset.MimeType, OwnerSubjectUUID: asset.OwnerSubjectUUID, Status: asset.UploadState, CreatedAt: asset.CreatedAt, UpdatedAt: asset.UpdatedAt}
}
func hostUUID(raw string) (string, error) {
	value, err := uuid.Parse(raw)
	if err != nil || value == uuid.Nil || value.String() != raw {
		return "", errors.New("media.invalid_uuid")
	}
	return value.String(), nil
}
func hostAPIKeyHash(c *gin.Context) string {
	raw, _ := c.Get("auth_api_key_hash")
	value, _ := raw.(string)
	return strings.TrimSpace(value)
}
func hostRejectTenantOverride(c *gin.Context) bool {
	for key := range c.Request.URL.Query() {
		if strings.Contains(strings.ToLower(key), "tenant") {
			dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(errors.New("media.tenant_override")))
			return false
		}
	}
	for key := range c.Request.Header {
		if strings.Contains(strings.ToLower(key), "tenant") {
			dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(errors.New("media.tenant_override")))
			return false
		}
	}
	return true
}
func hostMediaError(err error) error {
	switch {
	case errors.Is(err, mediasvc.ErrInvalidUploadMethod), errors.Is(err, mediasvc.ErrExternalURLRequired), errors.Is(err, mediasvc.ErrObjectKeyMustBeUUID), errors.Is(err, mediasvc.ErrContentSHA256Invalid), errors.Is(err, mediasvc.ErrContentSHA256Required):
		return mediasvc.MediaInvalidArgumentError(err)
	case errors.Is(err, mediasvc.ErrAssetNotFound):
		return mediasvc.MediaAssetNotFoundError(err)
	case errors.Is(err, mediasvc.ErrInvalidStatusTransition), errors.Is(err, mediasvc.ErrUploadTicketExpired):
		return mediasvc.MediaAssetStateInvalidError(err)
	case errors.Is(err, mediasvc.ErrUploadValidationFailed):
		return mediasvc.MediaUploadValidationFailedError(err)
	default:
		return mediasvc.MediaUpstreamDependencyError(err)
	}
}
