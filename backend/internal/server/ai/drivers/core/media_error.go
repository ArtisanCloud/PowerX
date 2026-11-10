package core

import (
	"encoding/json"
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"net/http"
	"strings"
)

// MediaHTTPError 保留可诊断字段；原生响应与带签名地址不进入公共错误。
func MediaHTTPError(status int, header http.Header, raw []byte) *dto.AppError {
	var payload struct {
		Code      any    `json:"code"`
		RequestID string `json:"request_id"`
		Error     *struct {
			Code  any `json:"code"`
			Param any `json:"param"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &payload)
	code := payload.Code
	if payload.Error != nil && payload.Error.Code != nil {
		code = payload.Error.Code
	}
	trace := payload.RequestID
	if trace == "" {
		trace = header.Get("X-Request-Id")
	}
	if trace == "" {
		trace = header.Get("X-Tt-Logid")
	}
	details := map[string]any{"upstream_status": status}
	if code != nil {
		details["upstream_code"] = strings.TrimSpace(fmt.Sprint(code))
	}
	if trace != "" {
		details["provider_trace_id"] = trace
	}
	if payload.Error != nil && payload.Error.Param != nil {
		details["parameter"] = payload.Error.Param
	}
	if status < 400 || status > 599 {
		status = 502
	}
	return &dto.AppError{HTTPCode: status, Code: "AI_PROVIDER_REJECTED", Message: "AI_PROVIDER_REJECTED", Details: details}
}
