package media

import "time"

type CreateAssetRequest struct {
	Name             string            `form:"name" json:"name" binding:"required"`
	Description      string            `form:"description" json:"description"`
	Driver           string            `form:"driver" json:"driver" binding:"required"`
	Bucket           string            `form:"bucket" json:"bucket"`
	BaseURL          string            `form:"baseUrl" json:"baseUrl"`
	Folder           string            `form:"folder" json:"folder"`
	OwnerSubjectType string            `form:"ownerSubjectType" json:"ownerSubjectType" binding:"required"`
	OwnerSubjectID   string            `form:"ownerSubjectId" json:"ownerSubjectId" binding:"required"`
	Tags             []string          `form:"tags" json:"tags"`
	UploadMethod     string            `form:"uploadMethod" json:"uploadMethod" binding:"omitempty,oneof=direct_upload external_link presign_upload"`
	ExternalURL      string            `form:"externalUrl" json:"externalUrl"`
	ObjectKey        string            `form:"objectKey" json:"objectKey"`
	SizeBytes        *int64            `form:"sizeBytes" json:"sizeBytes"`
	MimeType         string            `form:"mimeType" json:"mimeType"`
	Metadata         map[string]string `form:"metadata" json:"metadata"`
}

type ListAssetsRequest struct {
	Page             int      `form:"page"`
	PageSize         int      `form:"pageSize"`
	Keyword          string   `form:"keyword"`
	Driver           string   `form:"driver"`
	OwnerSubjectType string   `form:"ownerSubjectType"`
	OwnerSubjectID   string   `form:"ownerSubjectId"`
	Tags             []string `form:"tags"`
	BusinessStatus   []string `form:"businessStatus"`
}

type PresignRequest struct {
	Action           string `json:"action" binding:"required,oneof=upload download"`
	Method           string `json:"method" binding:"omitempty,oneof=GET PUT POST"`
	ExpiresInSeconds int64  `json:"expiresInSeconds"`
}

type CreateVariantRequest struct {
	Name      string            `json:"name"`
	Driver    string            `json:"driver"`
	Bucket    string            `json:"bucket"`
	BaseURL   string            `json:"baseUrl"`
	ObjectKey string            `json:"objectKey"`
	SizeBytes *int64            `json:"sizeBytes"`
	MimeType  string            `json:"mimeType"`
	Metadata  map[string]string `json:"metadata"`
}

type UpdateAssetRequest struct {
	Name           *string           `json:"name"`
	Description    *string           `json:"description"`
	BusinessStatus *string           `json:"businessStatus"`
	Tags           []string          `json:"tags"`
	Metadata       map[string]string `json:"metadata"`
}

type assetViewResponse struct {
	UUID              string     `json:"uuid"`
	TenantUUID        string     `json:"tenant_uuid"`
	Name              string     `json:"name"`
	Description       string     `json:"description,omitempty"`
	Driver            string     `json:"driver"`
	Folder            string     `json:"folder,omitempty"`
	ObjectKey         string     `json:"objectKey"`
	ExternalURL       string     `json:"externalUrl,omitempty"`
	SizeBytes         *int64     `json:"sizeBytes,omitempty"`
	MimeType          string     `json:"mimeType,omitempty"`
	OwnerSubjectType  string     `json:"ownerSubjectType,omitempty"`
	OwnerSubjectID    string     `json:"ownerSubjectId,omitempty"`
	Tags              []string   `json:"tags,omitempty"`
	BusinessStatus    string     `json:"businessStatus"`
	DownloadURL       string     `json:"downloadUrl,omitempty"`
	DownloadExpiredAt *time.Time `json:"downloadExpiredAt,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
}

type assetVariantViewResponse struct {
	UUID              string     `json:"uuid"`
	TenantUUID        string     `json:"tenant_uuid"`
	AssetUUID         string     `json:"assetUuid"`
	Variant           string     `json:"variant"`
	Name              string     `json:"name,omitempty"`
	Driver            string     `json:"driver"`
	ObjectKey         string     `json:"objectKey"`
	SizeBytes         *int64     `json:"sizeBytes,omitempty"`
	MimeType          string     `json:"mimeType,omitempty"`
	DownloadURL       string     `json:"downloadUrl,omitempty"`
	DownloadExpiredAt *time.Time `json:"downloadExpiredAt,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
}
