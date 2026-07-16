package metadata

type I18nMap map[string]string

type Display struct {
	DisplayName          string `json:"display_name"`
	DisplayDescription   string `json:"display_description,omitempty"`
	DisplayLocale        string `json:"display_locale"`
	DisplayLocaleMissing bool   `json:"display_locale_missing"`
}

type PaginationRequest struct {
	Page     int `form:"page" json:"page"`
	PageSize int `form:"page_size" json:"page_size"`
}

type PaginationResponse struct {
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Total    int64 `json:"total"`
}

type ReferenceSummary struct {
	ResourceType string `json:"resource_type"`
	ResourceUUID string `json:"resource_uuid"`
	FieldName    string `json:"field_name"`
	Count        int64  `json:"count"`
}

type ListResponse[T any] struct {
	Data       []T                `json:"data"`
	Pagination PaginationResponse `json:"pagination"`
}
