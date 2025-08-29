package errors

import (
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/server/mcp/types"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"net/http"
)

// MCPError MCP 错误接口
type MCPError interface {
	Error() string
	Code() string
	HTTPStatus() int
	Details() []schemas.ErrorField
	ToResponse() *schemas.ErrorResponse
}

// mcpError MCP 错误实现
type mcpError struct {
	code       string
	httpStatus int
	message    string
	details    []schemas.ErrorField
}

// Error 实现 error 接口
func (e *mcpError) Error() string {
	return e.message
}

// Code 返回错误码
func (e *mcpError) Code() string {
	return e.code
}

// HTTPStatus 返回HTTP状态码
func (e *mcpError) HTTPStatus() int {
	return e.httpStatus
}

// Details 返回错误详情
func (e *mcpError) Details() []schemas.ErrorField {
	return e.details
}

// ToResponse 转换为错误响应
func (e *mcpError) ToResponse() *schemas.ErrorResponse {
	return &schemas.ErrorResponse{
		Error: schemas.ErrorDetail{
			Code:       e.code,
			HTTPStatus: e.httpStatus,
			Message:    e.message,
			Details:    e.details,
		},
	}
}

// NewMCPError 创建新的MCP错误
func NewMCPError(code, message string, details ...schemas.ErrorField) MCPError {
	httpStatus, exists := types.ErrorCodeToHTTPStatus[code]
	if !exists {
		httpStatus = http.StatusInternalServerError
	}

	return &mcpError{
		code:       code,
		httpStatus: httpStatus,
		message:    message,
		details:    details,
	}
}

// InvalidArgument 创建无效参数错误
func InvalidArgument(message string, details ...schemas.ErrorField) MCPError {
	return NewMCPError(types.ErrorCodeInvalidArgument, message, details...)
}

// InvalidArgumentf 创建格式化的无效参数错误
func InvalidArgumentf(format string, args ...interface{}) MCPError {
	return InvalidArgument(fmt.Sprintf(format, args...))
}

// InvalidField 创建无效字段错误
func InvalidField(field, reason string) MCPError {
	return InvalidArgument(
		fmt.Sprintf("Invalid field '%s': %s", field, reason),
		schemas.ErrorField{Field: field, Reason: reason},
	)
}

// RequiredField 创建必需字段错误
func RequiredField(field string) MCPError {
	return InvalidField(field, "field is required")
}

// NotFound 创建未找到错误
func NotFound(message string, details ...schemas.ErrorField) MCPError {
	return NewMCPError(types.ErrorCodeNotFound, message, details...)
}

// NotFoundf 创建格式化的未找到错误
func NotFoundf(format string, args ...interface{}) MCPError {
	return NotFound(fmt.Sprintf(format, args...))
}

// ToolNotFound 创建工具未找到错误
func ToolNotFound(toolID string) MCPError {
	return NotFoundf("Tool '%s' not found", toolID)
}

// FlowNotFound 创建流程未找到错误
func FlowNotFound(flowID string) MCPError {
	return NotFoundf("Flow '%s' not found", flowID)
}

// BlueprintNotFound 创建蓝图未找到错误
func BlueprintNotFound(blueprintID string) MCPError {
	return NotFoundf("Blueprint '%s' not found", blueprintID)
}

// Unauthorized 创建未授权错误
func Unauthorized(message string, details ...schemas.ErrorField) MCPError {
	return NewMCPError(types.ErrorCodeUnauthorized, message, details...)
}

// Forbidden 创建禁止访问错误
func Forbidden(message string, details ...schemas.ErrorField) MCPError {
	return NewMCPError(types.ErrorCodeForbidden, message, details...)
}

// PermissionDenied 创建权限拒绝错误
func PermissionDenied(toolID, userRole string) MCPError {
	return Forbidden(
		fmt.Sprintf("Permission denied: role '%s' cannot access tool '%s'", userRole, toolID),
		schemas.ErrorField{Field: "user_role", Reason: "insufficient permissions"},
		schemas.ErrorField{Field: "tool_id", Reason: "access denied"},
	)
}

// Internal 创建内部错误
func Internal(message string, details ...schemas.ErrorField) MCPError {
	return NewMCPError(types.ErrorCodeInternal, message, details...)
}

// Internalf 创建格式化的内部错误
func Internalf(format string, args ...interface{}) MCPError {
	return Internal(fmt.Sprintf(format, args...))
}

// WrapError 包装标准错误为MCP错误
func WrapError(err error, code string) MCPError {
	if err == nil {
		return nil
	}

	// 如果已经是MCP错误，直接返回
	if mcpErr, ok := err.(MCPError); ok {
		return mcpErr
	}

	return NewMCPError(code, err.Error())
}

// WrapInternal 包装为内部错误
func WrapInternal(err error) MCPError {
	return WrapError(err, types.ErrorCodeInternal)
}

// ValidationError 验证错误构建器
type ValidationError struct {
	fields []schemas.ErrorField
}

// NewValidationError 创建验证错误构建器
func NewValidationError() *ValidationError {
	return &ValidationError{
		fields: make([]schemas.ErrorField, 0),
	}
}

// AddField 添加字段错误
func (v *ValidationError) AddField(field, reason string) *ValidationError {
	v.fields = append(v.fields, schemas.ErrorField{
		Field:  field,
		Reason: reason,
	})
	return v
}

// AddRequired 添加必需字段错误
func (v *ValidationError) AddRequired(field string) *ValidationError {
	return v.AddField(field, "field is required")
}

// AddInvalid 添加无效字段错误
func (v *ValidationError) AddInvalid(field, reason string) *ValidationError {
	return v.AddField(field, reason)
}

// HasErrors 检查是否有错误
func (v *ValidationError) HasErrors() bool {
	return len(v.fields) > 0
}

// ToError 转换为MCP错误
func (v *ValidationError) ToError() MCPError {
	if !v.HasErrors() {
		return nil
	}

	message := fmt.Sprintf("Validation failed with %d error(s)", len(v.fields))
	return InvalidArgument(message, v.fields...)
}

// ErrorHandler 错误处理器接口
type ErrorHandler interface {
	Handle(err error) MCPError
}

// DefaultErrorHandler 默认错误处理器
type DefaultErrorHandler struct{}

// Handle 处理错误
func (h *DefaultErrorHandler) Handle(err error) MCPError {
	if err == nil {
		return nil
	}

	// 如果已经是MCP错误，直接返回
	if mcpErr, ok := err.(MCPError); ok {
		return mcpErr
	}

	// 默认包装为内部错误
	return WrapInternal(err)
}

// 全局错误处理器
var globalErrorHandler ErrorHandler = &DefaultErrorHandler{}

// SetGlobalErrorHandler 设置全局错误处理器
func SetGlobalErrorHandler(handler ErrorHandler) {
	globalErrorHandler = handler
}

// HandleError 使用全局错误处理器处理错误
func HandleError(err error) MCPError {
	return globalErrorHandler.Handle(err)
}
