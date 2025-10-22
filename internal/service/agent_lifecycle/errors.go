package agent_lifecycle

import "errors"

var (
	// ErrAliasConflict 表示租户内代理别名冲突。
	ErrAliasConflict = errors.New("agent alias already exists in tenant")
	// ErrAgentNotFound 表示目标代理不存在。
	ErrAgentNotFound = errors.New("agent not found")
	// ErrInvalidStatusTransition 表示生命周期状态非法。
	ErrInvalidStatusTransition = errors.New("invalid lifecycle status transition")
)
