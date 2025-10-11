package media

// internal/transport/http/admin/media/dto.go

type CreateAssetRequest struct {
	Name             string         `json:"name" validate:"required"`
	Description      string         `json:"description"`
	Driver           string         `json:"driver"`
	Bucket           string         `json:"bucket"`
	BaseURL          string         `json:"baseUrl"`
	Folder           string         `json:"folder"`
	OwnerSubjectType string         `json:"ownerSubjectType"`
	OwnerSubjectID   string         `json:"ownerSubjectId"`
	Tags             []string       `json:"tags"`
	UploadMethod     string         `json:"uploadMethod" validate:"omitempty,oneof=direct_upload external_link presign_upload"`
	ExternalURL      string         `json:"externalUrl"`
	ObjectKey        string         `json:"objectKey"`
	SizeBytes        *int64         `json:"sizeBytes"`
	MimeType         string         `json:"mimeType"`
	Metadata         map[string]any `json:"metadata"`
}

type UpdateAssetRequest struct {
	Name           *string        `json:"name"`
	Description    *string        `json:"description"`
	BusinessStatus *string        `json:"businessStatus"`
	Tags           []string       `json:"tags"`
	Metadata       map[string]any `json:"metadata"`
}

type ListAssetsRequest struct {
	Page             int      `form:"page"`
	PageSize         int      `form:"pageSize"`
	Keyword          string   `form:"keyword"`
	Driver           string   `form:"driver"`
	OwnerSubjectType string   `form:"ownerSubjectType"`
	OwnerSubjectID   string   `form:"ownerSubjectId"`
	BusinessStatus   []string `form:"businessStatus"`
	Tags             []string `form:"tags"`
	UUIDs            []string `form:"uuid"`
	IncludeDeleted   bool     `form:"includeDeleted"`
	OnlyDeleted      bool     `form:"onlyDeleted"`
}

type PresignRequest struct {
	Action           string            `json:"action" validate:"required,oneof=upload download"`
	Method           string            `json:"method" validate:"omitempty,oneof=GET PUT POST"`
	ExpiresInSeconds int64             `json:"expiresInSeconds" validate:"omitempty,min=1"`
	ExpiresIn        int64             `json:"expires_in" validate:"omitempty,min=1"`
	Filename         string            `json:"filename"`
	ContentType      string            `json:"content_type"`
	Headers          map[string]string `json:"headers"`
	Metadata         map[string]string `json:"metadata"`
}
