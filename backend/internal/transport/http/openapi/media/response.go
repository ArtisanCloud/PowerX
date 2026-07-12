package media

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
)

func respondList(c *gin.Context, items any, total int64, page, pageSize int) {
	c.JSON(http.StatusOK, gin.H{
		"items":    items,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

func assetView(asset *mediasvc.Asset) assetViewResponse {
	var sizePtr *int64
	if asset.SizeBytes > 0 {
		value := asset.SizeBytes
		sizePtr = &value
	}
	var expiresAt *time.Time
	if asset.DownloadExpiry != nil {
		expiresAt = asset.DownloadExpiry
	}
	return assetViewResponse{
		UUID:              asset.UUID,
		TenantUUID:        asset.TenantUUID,
		Name:              asset.Name,
		Description:       asset.Description,
		Driver:            asset.Driver,
		Folder:            asset.Folder,
		ObjectKey:         asset.StorageKey,
		ExternalURL:       asset.ExternalURL,
		SizeBytes:         sizePtr,
		MimeType:          asset.MimeType,
		OwnerSubjectType:  asset.OwnerType,
		OwnerSubjectID:    asset.OwnerID,
		Tags:              asset.Tags,
		BusinessStatus:    asset.BusinessStatus,
		DownloadURL:       asset.DownloadURL,
		DownloadExpiredAt: expiresAt,
		CreatedAt:         asset.CreatedAt,
	}
}

func assetVariantView(variant *mediasvc.AssetVariant) assetVariantViewResponse {
	var sizePtr *int64
	if variant.SizeBytes > 0 {
		value := variant.SizeBytes
		sizePtr = &value
	}
	var expiresAt *time.Time
	if variant.DownloadExpiry != nil {
		expiresAt = variant.DownloadExpiry
	}
	return assetVariantViewResponse{
		UUID:              variant.UUID,
		TenantUUID:        variant.TenantUUID,
		AssetUUID:         variant.AssetUUID,
		Variant:           variant.Variant,
		Name:              variant.Name,
		Driver:            variant.Driver,
		ObjectKey:         variant.StorageKey,
		SizeBytes:         sizePtr,
		MimeType:          variant.MimeType,
		DownloadURL:       variant.DownloadURL,
		DownloadExpiredAt: expiresAt,
		CreatedAt:         variant.CreatedAt,
	}
}
