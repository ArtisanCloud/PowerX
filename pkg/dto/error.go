// pkg/dto/error.go
package dto

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// AppError 是业务统一错误类型，用于在 Service/Repo 层向上抛出，Handler 再统一转成 HTTP 响应。
// - HTTPCode: 建议使用 net/http 标准状态码
// - Message:  给前端/用户看的错误摘要（中文）
// - Err:      内部原始错误（不会直接暴露给前端，但便于日志/追踪）
// - Details:  可选的结构化上下文（比如字段名、资源ID等）
type AppError struct {
	HTTPCode int
	Message  string
	Err      error
	Details  map[string]interface{}
}

func (e *AppError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *AppError) Unwrap() error { return e.Err }

// -------- 基础构造 --------

// NewError 生成通用 AppError
func NewError(code int, message string, err error) *AppError {
	return &AppError{HTTPCode: code, Message: message, Err: err}
}

// Wrap 在已有错误基础上包一层（保持 code/message）
func Wrap(err error, message string) *AppError {
	if err == nil {
		return nil
	}
	var ae *AppError
	if errors.As(err, &ae) {
		// 叠加信息：保留下层 code，替换更友好的 message
		return &AppError{
			HTTPCode: ae.HTTPCode,
			Message:  message,
			Err:      ae,
			Details:  ae.Details,
		}
	}
	return &AppError{HTTPCode: http.StatusInternalServerError, Message: message, Err: err}
}

// WithDetails 为 AppError 附加结构化明细
func WithDetails(err error, kv map[string]interface{}) *AppError {
	if err == nil {
		return nil
	}
	var ae *AppError
	if errors.As(err, &ae) {
		if ae.Details == nil {
			ae.Details = map[string]interface{}{}
		}
		for k, v := range kv {
			ae.Details[k] = v
		}
		return ae
	}
	return &AppError{
		HTTPCode: http.StatusInternalServerError,
		Message:  "内部错误",
		Err:      err,
		Details:  kv,
	}
}

// -------- 语义化构造（便捷函数） --------

func NewBadRequest(message string, err error) *AppError {
	return NewError(http.StatusBadRequest, message, err)
}
func NewUnauthorized(message string, err error) *AppError {
	return NewError(http.StatusUnauthorized, message, err)
}
func NewForbidden(message string, err error) *AppError {
	return NewError(http.StatusForbidden, message, err)
}
func NewNotFound(message string, err error) *AppError {
	return NewError(http.StatusNotFound, message, err)
}
func NewConflict(message string, err error) *AppError {
	return NewError(http.StatusConflict, message, err)
}
func NewTooManyRequests(message string, err error) *AppError {
	return NewError(http.StatusTooManyRequests, message, err)
}
func NewInternal(message string, err error) *AppError {
	return NewError(http.StatusInternalServerError, message, err)
}

// -------- 查询辅助 --------

// StatusCode 从 error 提取 HTTP 状态码，默认 500
func StatusCode(err error) int {
	var ae *AppError
	if errors.As(err, &ae) && ae.HTTPCode > 0 {
		return ae.HTTPCode
	}
	return http.StatusInternalServerError
}

// MessageOf 从 error 提取面向前端的 Message，默认 "内部错误"
func MessageOf(err error) string {
	var ae *AppError
	if errors.As(err, &ae) && ae.Message != "" {
		return ae.Message
	}
	if err != nil {
		return "内部错误"
	}
	return ""
}

// DetailsOf 从 error 提取 Details
func DetailsOf(err error) map[string]interface{} {
	var ae *AppError
	if errors.As(err, &ae) && ae.Details != nil {
		return ae.Details
	}
	return nil
}

// -------- Handler 层响应辅助 --------

// RespondErrorFrom 将任意 error 统一转成标准 JSON 响应
// - 若是 AppError：用其 code/message；同时保留 err.Error() 到 Error 字段（方便排查）
// - 若是普通 error：500 + "内部错误"
func RespondErrorFrom(c *gin.Context, err error) {
	if err == nil {
		ResponseError(c, http.StatusOK, "success", nil)
		return
	}
	code := StatusCode(err)
	msg := MessageOf(err)
	// 复用你已有的 ResponseError（会把 err.Error() 放到 "error" 字段）
	ResponseError(c, code, msg, err)
}

// MustOK 语法糖：err==nil 则返回成功，否则按上面逻辑返回错误
func MustOK(c *gin.Context, data interface{}, err error) {
	if err != nil {
		RespondErrorFrom(c, err)
		return
	}
	ResponseSuccess(c, data)
}
