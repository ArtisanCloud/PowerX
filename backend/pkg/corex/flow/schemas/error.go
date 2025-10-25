package schemas

// ErrorResponse 统一错误格式
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail 错误详情
type ErrorDetail struct {
	Code       string       `json:"code"`
	HTTPStatus int          `json:"http_status"`
	Message    string       `json:"message"`
	Details    []ErrorField `json:"details,omitempty"`
}

// ErrorField 字段级错误信息
type ErrorField struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}
