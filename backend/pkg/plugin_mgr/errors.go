package plugin_mgr

import (
	"errors"
	"fmt"
	"net/http"
)

type Code string

const (
	// 校验/契约
	CodeInvalidManifest      Code = "invalid_manifest"
	CodeUnsupportedRuntime   Code = "unsupported_runtime"
	CodeMissingFile          Code = "missing_file"
	CodeChecksumMismatch     Code = "checksum_mismatch"
	CodeSignatureInvalid     Code = "signature_invalid"
	CodeIncompatibleCoreXVer Code = "incompatible_corex_version"
	CodeInvalidArg           Code = "invalid_arg"     // 参数不合法
	CodeAlreadyExists        Code = "already_exists"  // 资源已存在（例如版本已经安装）
	CodeLifecycleError       Code = "lifecycle_error" // 启停/进程/挂载等生命周期相关错误

	// 状态/版本
	CodeAlreadyInstalled Code = "already_installed"
	CodeAlreadyEnabled   Code = "already_enabled"
	CodeAlreadyDisabled  Code = "already_disabled"
	CodeNotInstalled     Code = "not_installed"
	CodeVersionConflict  Code = "version_conflict"

	// 运行期
	CodeProcessStartFailed Code = "process_start_failed"
	CodeHealthcheckFailed  Code = "healthcheck_failed"
	CodeRouteMountFailed   Code = "route_mount_failed"
	CodeMigrationFailed    Code = "migration_failed"

	// 基础设施
	CodeRegistryError Code = "registry_error"
	CodeIOError       Code = "io_error"
	CodeTimeout       Code = "timeout"

	// 通用
	CodeUnauthorized Code = "unauthorized"
	CodeForbidden    Code = "forbidden"
	CodeNotFound     Code = "not_found"
	CodeConflict     Code = "conflict"
	CodeInternal     Code = "internal"
)

type ManagerError struct {
	Code     Code
	Op       string // install/enable/disable/upgrade/scan/...
	PluginID string
	Version  string
	Path     string
	Msg      string
	Cause    error
}

func (e *ManagerError) Error() string {
	idver := e.PluginID
	if e.Version != "" {
		if idver != "" {
			idver += "@"
		}
		idver += e.Version
	}
	s := ""
	if e.Op != "" {
		s += e.Op + ": "
	}
	s += string(e.Code)
	if idver != "" {
		s += " [" + idver + "]"
	}
	if e.Path != "" {
		s += " (" + e.Path + ")"
	}
	if e.Msg != "" {
		s += ": " + e.Msg
	}
	if e.Cause != nil {
		s += ": " + e.Cause.Error()
	}
	return s
}
func (e *ManagerError) Unwrap() error { return e.Cause }
func (e *ManagerError) Is(target error) bool {
	t, ok := target.(*ManagerError)
	return ok && e.Code == t.Code
}

type ErrOption func(*ManagerError)

func WithOp(op string) ErrOption       { return func(e *ManagerError) { e.Op = op } }
func WithPlugin(id string) ErrOption   { return func(e *ManagerError) { e.PluginID = id } }
func WithVersion(ver string) ErrOption { return func(e *ManagerError) { e.Version = ver } }
func WithPath(p string) ErrOption      { return func(e *ManagerError) { e.Path = p } }
func WithMsg(format string, a ...any) ErrOption {
	return func(e *ManagerError) { e.Msg = fmt.Sprintf(format, a...) }
}
func WithCause(err error) ErrOption { return func(e *ManagerError) { e.Cause = err } }

func NewError(code Code, opts ...ErrOption) *ManagerError {
	e := &ManagerError{Code: code}
	for _, opt := range opts {
		opt(e)
	}
	return e
}
func Wrap(code Code, cause error, opts ...ErrOption) *ManagerError {
	opts = append(opts, WithCause(cause))
	return NewError(code, opts...)
}
func IsCode(err error, code Code) bool {
	var me *ManagerError
	if errors.As(err, &me) {
		return me.Code == code
	}
	return false
}
func CodeOf(err error) Code {
	var me *ManagerError
	if errors.As(err, &me) {
		return me.Code
	}
	return CodeInternal
}
func HTTPStatusOf(code Code) int {
	switch code {
	case CodeInvalidManifest, CodeUnsupportedRuntime, CodeMissingFile,
		CodeChecksumMismatch, CodeSignatureInvalid, CodeIncompatibleCoreXVer:
		return http.StatusBadRequest
	case CodeAlreadyInstalled, CodeAlreadyEnabled, CodeAlreadyDisabled,
		CodeVersionConflict, CodeConflict:
		return http.StatusConflict
	case CodeNotInstalled, CodeNotFound:
		return http.StatusNotFound
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeForbidden:
		return http.StatusForbidden
	case CodeTimeout:
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}

// 字段级校验错误（Validate使用）
type FieldError struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}
type ValidationError struct {
	Op       string       `json:"op"`
	PluginID string       `json:"plugin_id"`
	Version  string       `json:"version"`
	Fields   []FieldError `json:"fields"`
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed (%d field errors)", len(e.Fields))
}
func (e *ValidationError) ToManagerError() *ManagerError {
	return &ManagerError{
		Code:     CodeInvalidManifest,
		Op:       e.Op,
		PluginID: e.PluginID,
		Version:  e.Version,
		Msg:      e.Error(),
		Cause:    e,
	}
}
