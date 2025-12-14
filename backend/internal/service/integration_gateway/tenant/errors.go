package tenant

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ErrRouteNotAccessible 表示路由不存在或不可访问。
type ErrRouteNotAccessible struct {
	Slug     string
	TenantUUID string
}

func (e ErrRouteNotAccessible) Error() string {
	return fmt.Sprintf("integration gateway: route %s inaccessible for tenant %s", e.Slug, e.TenantUUID)
}

// ErrChannelDisabled 表示指定通道未启用。
type ErrChannelDisabled struct {
	RouteID uuid.UUID
	Channel string
}

func (e ErrChannelDisabled) Error() string {
	return fmt.Sprintf("integration gateway: channel %s disabled for route %s", e.Channel, e.RouteID)
}

// ErrToolGrantDenied 表示 Tool Grant 校验失败。
type ErrToolGrantDenied struct {
	RouteID  uuid.UUID
	TenantUUID string
	Grants   []string
	Reason   string
}

func (e ErrToolGrantDenied) Error() string {
	return fmt.Sprintf("integration gateway: tool grant denied for tenant %s route %s", e.TenantUUID, e.RouteID)
}

// RateLimitError 封装限流失败的元信息。
type RateLimitError struct {
	RouteID    uuid.UUID
	Scope      string
	RetryAfter time.Duration
	Remaining  int64
}

func (e RateLimitError) Error() string {
	return fmt.Sprintf("integration gateway: rate limit exceeded (scope=%s retry_after=%s)", e.Scope, e.RetryAfter)
}
