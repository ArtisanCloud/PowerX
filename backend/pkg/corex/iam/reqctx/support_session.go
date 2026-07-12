package reqctx

import "context"

const (
	KeySupportSessionID         ctxKey = "corex.support_session_id"
	KeySupportSessionTargetUUID ctxKey = "corex.support_session_target_tenant_uuid"
	KeySupportSessionMode       ctxKey = "corex.support_session_mode"
)

func WithSupportSession(ctx context.Context, id uint64, targetTenantUUID, mode string) context.Context {
	if id > 0 {
		ctx = context.WithValue(ctx, KeySupportSessionID, id)
	}
	if targetTenantUUID != "" {
		ctx = context.WithValue(ctx, KeySupportSessionTargetUUID, targetTenantUUID)
	}
	if mode != "" {
		ctx = context.WithValue(ctx, KeySupportSessionMode, mode)
	}
	return ctx
}

func GetSupportSessionID(ctx context.Context) uint64 {
	if v, ok := ctx.Value(KeySupportSessionID).(uint64); ok {
		return v
	}
	return 0
}

func GetSupportSessionTargetTenantUUID(ctx context.Context) string {
	if v, ok := ctx.Value(KeySupportSessionTargetUUID).(string); ok {
		return v
	}
	return ""
}

func GetSupportSessionMode(ctx context.Context) string {
	if v, ok := ctx.Value(KeySupportSessionMode).(string); ok {
		return v
	}
	return ""
}
