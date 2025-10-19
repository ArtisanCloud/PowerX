package shared

import "errors"

const (
	// ErrorNamespace 统一错误码命名空间。
	ErrorNamespace = "event_fabric"

	// ErrorCodeTenantMismatch 租户上下文缺失或不匹配。
	ErrorCodeTenantMismatch = "event_fabric.tenant_mismatch"

	// ErrorCodeUnauthorized 未通过 ACL 校验。
	ErrorCodeUnauthorized = "event_fabric.unauthorized"

	// ErrorCodeAckTimeout 超过 ACK 超时时间。
	ErrorCodeAckTimeout = "event_fabric.ack_timeout"

	// ErrorCodeRetryExhausted 超过最大重试次数。
	ErrorCodeRetryExhausted = "event_fabric.retry_exhausted"
)

var (
	// ErrTenantMismatch 表示上下文缺少租户或租户不合法。
	ErrTenantMismatch = errors.New("event_fabric: tenant mismatch")

	// ErrUnauthorized 表示调用方未通过 ACL 校验。
	ErrUnauthorized = errors.New("event_fabric: unauthorized access")

	// ErrAckTimeout 表示订阅者未在约定时间内确认。
	ErrAckTimeout = errors.New("event_fabric: ack timeout exceeded")

	// ErrRetryExhausted 表示重试次数已耗尽。
	ErrRetryExhausted = errors.New("event_fabric: retry attempts exhausted")
)
