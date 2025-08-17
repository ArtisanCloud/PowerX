package dto

import "github.com/ArtisanCloud/PowerX/pkg/dynamic_form/model"

// ValidateFormRequest 用于表单验证 API
type ValidateFormRequest struct {
	FormID string                 `json:"form_id"`
	Input  map[string]interface{} `json:"input"`
}

// SubmitFormRequest 用于表单提交 API
type SubmitFormRequest struct {
	FormID string                 `json:"form_id"`
	Input  map[string]interface{} `json:"input"`
	// 可选：是否触发 side-effect，比如直接调用某个 tool
	Trigger string `json:"trigger,omitempty"`
}

type CreateFormRequest struct {
	*model.FormSchema
}

// FormCreateResponse 返回给调用方的 DTO
type FormCreateResponse struct {
	*model.FormSchema
}

type GetFormRequest struct {
	ID string `json:"id"`
}

type GetFormResponse struct {
	*model.FormSchema
}

type UpdateFormRequest struct {
	*model.FormSchema
}

type UpdateFormResponse struct {
	*model.FormSchema
}
