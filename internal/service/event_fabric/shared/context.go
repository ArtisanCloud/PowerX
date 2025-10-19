package shared

import (
	"context"
	"strings"
)

// TenantFromContext 读取上下文中的租户标识。
func TenantFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(ContextTenantKey).(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// SubscriberFromContext 读取上下文中的订阅者标识。
func SubscriberFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(ContextSubscriberKey).(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// TopicFilterFromContext 返回订阅过滤的 Topic 全名集合，忽略大小写。
func TopicFilterFromContext(ctx context.Context) map[string]struct{} {
	if ctx == nil {
		return nil
	}
	raw, ok := ctx.Value(ContextTopicsKey).(map[string]struct{})
	if !ok || len(raw) == 0 {
		return nil
	}
	result := make(map[string]struct{}, len(raw))
	for topic := range raw {
		if trimmed := strings.TrimSpace(topic); trimmed != "" {
			result[strings.ToLower(trimmed)] = struct{}{}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
