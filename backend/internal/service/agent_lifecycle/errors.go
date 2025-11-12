package agent_lifecycle

import "errors"

var (
	// ErrAliasConflict 表示租户内代理别名冲突。
	ErrAliasConflict = errors.New("agent alias already exists in tenant")
	// ErrAgentNotFound 表示目标代理不存在。
	ErrAgentNotFound = errors.New("agent not found")
	// ErrInvalidStatusTransition 表示生命周期状态非法。
	ErrInvalidStatusTransition = errors.New("invalid lifecycle status transition")
	// ErrInvalidCapacity 表示容量目标非法。
	ErrInvalidCapacity = errors.New("invalid capacity target")
	// ErrCapacityExceeded 表示超出最大容量限制。
	ErrCapacityExceeded = errors.New("capacity exceeds maximum limit")
	// ErrInvalidSubscription 表示订阅配置非法。
	ErrInvalidSubscription = errors.New("invalid subscription config")
	// ErrInvalidManifestPayload 表示 manifest 载荷非法或缺失字段。
	ErrInvalidManifestPayload = errors.New("invalid manifest payload")
	// ErrInvalidManifestSignature 表示 manifest 签名校验失败。
	ErrInvalidManifestSignature = errors.New("invalid manifest signature")
	// ErrSandboxExecutionFailed 表示沙箱执行失败。
	ErrSandboxExecutionFailed = errors.New("sandbox execution failed")
	// ErrPolicyConflict 表示租户表单触发策略冲突。
	ErrPolicyConflict = errors.New("tenant agent policy conflict")
	// ErrTenantFormNotFound 表示租户表单不存在。
	ErrTenantFormNotFound = errors.New("tenant agent form not found")
	// ErrTenantFormInvalidStatus 表示当前状态不允许该操作。
	ErrTenantFormInvalidStatus = errors.New("tenant agent form status invalid")
)

// PolicyConflict 描述策略冲突详情。
type PolicyConflict struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// PolicyConflictError 用于返回策略冲突集合。
type PolicyConflictError struct {
	Conflicts []PolicyConflict
}

func (e *PolicyConflictError) Error() string {
	return "tenant agent form hits policy conflict"
}

// Unwrap 允许 errors.Is(err, ErrPolicyConflict) 为 true。
func (e *PolicyConflictError) Unwrap() error {
	return ErrPolicyConflict
}

// NewPolicyConflictError 构造带详细信息的策略冲突错误。
func NewPolicyConflictError(conflicts []PolicyConflict) error {
	return &PolicyConflictError{Conflicts: conflicts}
}
