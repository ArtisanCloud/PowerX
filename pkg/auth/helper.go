package auth

import "context"

type ctxKey string

const (
	TenantIDKey  ctxKey = "tenant_id"  // 租户ID键
	SubjectKey   ctxKey = "subject"    // 主体键
	ScopeKey     ctxKey = "scope"      // 权限范围键
	AudienceKey  ctxKey = "audience"   // 受众键
	PlatformKey  ctxKey = "platform"   // 平台键
	TraceIDKey   ctxKey = "trace_id"   // 追踪ID键
	JWTClaimsKey ctxKey = "jwt_claims" // JWT声明键
)

// GetTenantID 从上下文获取租户ID
func GetTenantID(ctx context.Context) string {
	if v, ok := ctx.Value(TenantIDKey).(string); ok {
		return v
	}
	return ""
}

// GetTraceID 从上下文获取追踪ID
func GetTraceID(ctx context.Context) string {
	if v, ok := ctx.Value(TraceIDKey).(string); ok {
		return v
	}
	return ""
}

// GetSubject 从上下文获取主体
func GetSubject(ctx context.Context) string {
	if v, ok := ctx.Value(SubjectKey).(string); ok {
		return v
	}
	return ""
}

// GetScope 从上下文获取权限范围
func GetScope(ctx context.Context) string {
	if v, ok := ctx.Value(ScopeKey).(string); ok {
		return v
	}
	return ""
}

// GetPlatform 从上下文获取平台标识
func GetPlatform(ctx context.Context) string {
	if v, ok := ctx.Value(PlatformKey).(string); ok {
		return v
	}
	return ""
}

// GetJWTClaims 从上下文获取JWT声明
func GetJWTClaims(ctx context.Context) *CoreXClaims {
	if v, ok := ctx.Value(JWTClaimsKey).(*CoreXClaims); ok {
		return v
	}
	return nil
}
