package event_bus

import (
	"context"
	"strings"
)

// tenantUUIDFromContext 先读新的 tenant_uuid，上游兼容老 key: tenant_id。
func tenantUUIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if uuid, ok := ctx.Value("tenant_uuid").(string); ok {
		if trimmed := strings.TrimSpace(uuid); trimmed != "" {
			return trimmed
		}
	}
	if legacy, ok := ctx.Value("tenant_id").(string); ok {
		return strings.TrimSpace(legacy)
	}
	return ""
}
