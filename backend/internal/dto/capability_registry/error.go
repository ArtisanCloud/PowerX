package capabilityregistrydto

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorCode 为能力目录/集成网关暴露给客户端的统一错误码。
type ErrorCode string

const (
	ErrorInvalidRequest      ErrorCode = "registry.invalid_request"
	ErrorInvalidPayload      ErrorCode = "registry.invalid_payload"
	ErrorNotFound            ErrorCode = "registry.not_found"
	ErrorVersionConflict     ErrorCode = "registry.version_conflict"
	ErrorVersionRequired     ErrorCode = "registry.version_required"
	ErrorVersionLocked       ErrorCode = "registry.version_locked"
	ErrorInternal            ErrorCode = "registry.internal_error"
	ErrorUnavailable         ErrorCode = "registry.unavailable"
	ErrorTenantUUIDMissing   ErrorCode = "tenant.uuid_missing"
	ErrorTenantUUIDInvalid   ErrorCode = "tenant.uuid_invalid"
	ErrorTenantMismatch      ErrorCode = "tenant.mismatch"
	ErrorInvokeFailed        ErrorCode = "integration.invoke_failed"
	ErrorCapabilityForbidden ErrorCode = "registry.capability_forbidden"
	ErrorSafeModeActive      ErrorCode = "tenant.safe_mode_active"
	ErrorFeatureFlagMissing  ErrorCode = "registry.feature_flag_required"
	ErrorToolGrantMissing    ErrorCode = "registry.tool_grant_required"
)

// ErrorTemplate 描述一次标准化错误响应。
type ErrorTemplate struct {
	HTTPStatus     int
	GRPCStatus     codes.Code
	Code           ErrorCode
	Hint           string
	NextSteps      []string
	ManualUpgrade  bool
	defaultDetails map[string]interface{}
}

// Respond 将错误按照统一 envelope 返回给 HTTP 调用方。
func (t ErrorTemplate) Respond(c *gin.Context, err error, extras ...map[string]interface{}) {
	if c == nil {
		return
	}
	dto.ResponseErrorWithDetails(c, t.HTTPStatus, string(t.Code), err, t.details(extras...))
}

// GRPCError 将错误转换为 gRPC status。
func (t ErrorTemplate) GRPCError(err error) error {
	msgParts := []string{string(t.Code)}
	if hint := strings.TrimSpace(t.Hint); hint != "" {
		msgParts = append(msgParts, hint)
	}
	if err != nil {
		msgParts = append(msgParts, err.Error())
	}
	return status.Error(t.GRPCStatus, strings.Join(msgParts, ": "))
}

func (t ErrorTemplate) details(extras ...map[string]interface{}) map[string]interface{} {
	details := map[string]interface{}{
		"code": string(t.Code),
	}
	if hint := strings.TrimSpace(t.Hint); hint != "" {
		details["hint"] = hint
	}
	if len(t.NextSteps) > 0 {
		details["next_steps"] = t.NextSteps
	}
	if t.ManualUpgrade {
		details["manual_upgrade_required"] = true
	}
	for _, extra := range extras {
		for k, v := range extra {
			details[k] = v
		}
	}
	for k, v := range t.defaultDetails {
		if _, exists := details[k]; !exists {
			details[k] = v
		}
	}
	return details
}

// WithHint 复制模板并覆盖 hint。
func (t ErrorTemplate) WithHint(hint string) ErrorTemplate {
	t.Hint = hint
	return t
}

// WithDetails 复制模板并附加默认 details。
func (t ErrorTemplate) WithDetails(details map[string]interface{}) ErrorTemplate {
	if len(details) == 0 {
		return t
	}
	if t.defaultDetails == nil {
		t.defaultDetails = map[string]interface{}{}
	}
	for k, v := range details {
		t.defaultDetails[k] = v
	}
	return t
}

// RespondError 是 Respond 的便捷封装。
func RespondError(c *gin.Context, tpl ErrorTemplate, err error, extras ...map[string]interface{}) {
	tpl.Respond(c, err, extras...)
}

// ToGRPCError 是 GRPCError 的便捷封装。
func ToGRPCError(tpl ErrorTemplate, err error) error {
	return tpl.GRPCError(err)
}

var manualUpgradeSteps = []string{
	"调用 GET /admin/capability-registry/capabilities/{capabilityId}/tenants/{tenant_uuid} 刷新最新版本",
	"若 Workflow Builder 显示“需手动升级”，请在 Admin UI 或 CLI 中执行升级后重试",
}

// 预定义错误模板，供 HTTP/gRPC Handler 复用。
var (
	ErrInvalidRequest = ErrorTemplate{
		HTTPStatus: http.StatusBadRequest,
		GRPCStatus: codes.InvalidArgument,
		Code:       ErrorInvalidRequest,
		Hint:       "请求缺少必填字段或格式不合法",
	}
	ErrInvalidPayload = ErrorTemplate{
		HTTPStatus: http.StatusUnprocessableEntity,
		GRPCStatus: codes.InvalidArgument,
		Code:       ErrorInvalidPayload,
		Hint:       "能力声明或 Adapter 定义无效",
	}
	ErrNotFound = ErrorTemplate{
		HTTPStatus: http.StatusNotFound,
		GRPCStatus: codes.NotFound,
		Code:       ErrorNotFound,
		Hint:       "目标能力或记录不存在",
	}
	ErrVersionConflict = ErrorTemplate{
		HTTPStatus:    http.StatusPreconditionFailed,
		GRPCStatus:    codes.FailedPrecondition,
		Code:          ErrorVersionConflict,
		Hint:          "版本号已过期，请刷新能力详情后重试",
		NextSteps:     manualUpgradeSteps,
		ManualUpgrade: true,
	}
	ErrVersionRequired = ErrorTemplate{
		HTTPStatus:    http.StatusPreconditionRequired,
		GRPCStatus:    codes.FailedPrecondition,
		Code:          ErrorVersionRequired,
		Hint:          "缺少 If-Match 或 version 字段，无法执行幂等更新",
		NextSteps:     manualUpgradeSteps,
		ManualUpgrade: true,
	}
	ErrVersionLocked = NewManualUpgradeError(
		ErrorVersionLocked,
		http.StatusFailedDependency,
		codes.FailedPrecondition,
		"能力版本需管理员确认升级后才能调用",
	)
	ErrInternal = ErrorTemplate{
		HTTPStatus: http.StatusInternalServerError,
		GRPCStatus: codes.Internal,
		Code:       ErrorInternal,
		Hint:       "服务内部异常，请稍后重试或联系管理员",
	}
	ErrUnavailable = ErrorTemplate{
		HTTPStatus: http.StatusServiceUnavailable,
		GRPCStatus: codes.Unavailable,
		Code:       ErrorUnavailable,
		Hint:       "依赖服务临时不可用，请稍后重试",
	}
	ErrTenantUUIDMissing = ErrorTemplate{
		HTTPStatus: http.StatusUnauthorized,
		GRPCStatus: codes.Unauthenticated,
		Code:       ErrorTenantUUIDMissing,
		Hint:       "请求头或参数缺少 tenant_uuid",
	}
	ErrTenantUUIDInvalid = ErrorTemplate{
		HTTPStatus: http.StatusBadRequest,
		GRPCStatus: codes.InvalidArgument,
		Code:       ErrorTenantUUIDInvalid,
		Hint:       "tenant_uuid 格式错误",
	}
	ErrTenantMismatch = ErrorTemplate{
		HTTPStatus: http.StatusForbidden,
		GRPCStatus: codes.PermissionDenied,
		Code:       ErrorTenantMismatch,
		Hint:       "请求主体与凭证中的 tenant_uuid 不一致",
	}
	ErrCapabilityForbidden = ErrorTemplate{
		HTTPStatus: http.StatusForbidden,
		GRPCStatus: codes.PermissionDenied,
		Code:       ErrorCapabilityForbidden,
		Hint:       "能力未授权或已被租户禁用",
	}
	ErrSafeModeActive = ErrorTemplate{
		HTTPStatus: http.StatusServiceUnavailable,
		GRPCStatus: codes.Unavailable,
		Code:       ErrorSafeModeActive,
		Hint:       "租户处于 Safe-mode，暂时无法调用该能力",
	}.WithDetails(map[string]interface{}{
		"next_steps": []string{
			"请前往 Agent Model Hub 或联系运维解除 Safe-mode 后重试",
		},
	})
	ErrFeatureFlagMissing = ErrorTemplate{
		HTTPStatus: http.StatusForbidden,
		GRPCStatus: codes.PermissionDenied,
		Code:       ErrorFeatureFlagMissing,
		Hint:       "缺少启用该能力所需的 Feature Flag",
	}
	ErrToolGrantMissing = ErrorTemplate{
		HTTPStatus: http.StatusForbidden,
		GRPCStatus: codes.PermissionDenied,
		Code:       ErrorToolGrantMissing,
		Hint:       "缺少调用该能力所需的 Tool Grant",
	}
	ErrInvokeFailed = ErrorTemplate{
		HTTPStatus: http.StatusBadGateway,
		GRPCStatus: codes.Internal,
		Code:       ErrorInvokeFailed,
		Hint:       "能力调用失败，请参考 trace_id 排查",
	}.WithDetails(map[string]interface{}{
		"next_steps": []string{
			"检查事件总线与 Plugin 状态，确保协议通道可用",
			"若错误提示手动升级，请在 Workflow/Admin 控制台确认模板升级",
		},
	})
)

// WrapError 根据实际错误选择模板（若无匹配则返回 ErrInternal）。
func WrapError(err error, candidates ...struct {
	Check    func(error) bool
	Template ErrorTemplate
}) ErrorTemplate {
	for _, candidate := range candidates {
		if candidate.Check == nil {
			continue
		}
		if candidate.Check(err) {
			return candidate.Template
		}
	}
	return ErrInternal
}

// NewManualUpgradeError 生成包含手动升级提示的错误模板。
func NewManualUpgradeError(code ErrorCode, status int, grpc codes.Code, hint string) ErrorTemplate {
	return ErrorTemplate{
		HTTPStatus:    status,
		GRPCStatus:    grpc,
		Code:          code,
		Hint:          hint,
		NextSteps:     manualUpgradeSteps,
		ManualUpgrade: true,
	}
}

// FormatDetail 提供一个便捷方法返回 map，用于 Respond 的额外 details。
func FormatDetail(key string, value interface{}) map[string]interface{} {
	return map[string]interface{}{key: value}
}

// CombineDetails 合并多个 detail map。
func CombineDetails(values ...map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{}
	for _, kv := range values {
		for k, v := range kv {
			result[k] = v
		}
	}
	return result
}

// String implements fmt.Stringer for ErrorTemplate to assist logging.
func (t ErrorTemplate) String() string {
	return fmt.Sprintf("%s (http=%d grpc=%s)", t.Code, t.HTTPStatus, t.GRPCStatus.String())
}
