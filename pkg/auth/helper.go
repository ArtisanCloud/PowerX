// pkg/auth/helper.go
package auth

import "context"

type ctxKey string

const (
	TenantIDKey   ctxKey = "tenant_id"   // 租户ID键（uint64）
	TenantUUIDKey ctxKey = "tenant_uuid" // 租户UUID键（string）
	SubjectKey    ctxKey = "subject"     // 主体键（string）
	ScopeKey      ctxKey = "scope"       // 权限范围键（string）
	AudienceKey   ctxKey = "audience"    // 受众键（string）
	PlatformKey   ctxKey = "platform"    // 平台键（string）
	TraceIDKey    ctxKey = "trace_id"    // 追踪ID键（string）
	JWTClaimsKey  ctxKey = "jwt_claims"  // JWT声明键（*CoreXClaims）
	rootCtxKey    ctxKey = "is_root_ctx"
)

/*************** Getters：从 context 中读取 ***************/

func WithIsRoot(ctx context.Context, isRoot bool) context.Context {
	return context.WithValue(ctx, rootCtxKey, isRoot)
}

// GetTenantID 从上下文获取租户ID（优先 context 写入，其次可从 claims 兜底）
func GetTenantID(ctx context.Context) uint64 {
	if v, ok := ctx.Value(TenantIDKey).(uint64); ok {
		return v
	}
	if c := GetJWTClaims(ctx); c != nil && c.TenantID != 0 {
		return c.TenantID
	}
	return 0
}

// GetTenantUUID 从上下文获取租户UUID（修复：之前误用 TenantIDKey）
func GetTenantUUID(ctx context.Context) string {
	if v, ok := ctx.Value(TenantUUIDKey).(string); ok {
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

// GetAudience 从上下文获取受众
func GetAudience(ctx context.Context) string {
	if v, ok := ctx.Value(AudienceKey).(string); ok {
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

// GetJWTClaims 从上下文获取 JWT 声明
func GetJWTClaims(ctx context.Context) *CoreXClaims {
	if v, ok := ctx.Value(JWTClaimsKey).(*CoreXClaims); ok {
		return v
	}
	return nil
}

/*************** Claims 派生字段：从 claims 读取 ***************/

// GetUserID 从 claims 获取 UserID
func GetUserID(ctx context.Context) uint64 {
	if c := GetJWTClaims(ctx); c != nil {
		return c.UserID
	}
	return 0
}

// GetMemberID 从 claims 获取 MemberID（用户在当前租户的成员ID）
func GetMemberID(ctx context.Context) uint64 {
	if c := GetJWTClaims(ctx); c != nil {
		return c.MemberID
	}
	return 0
}

/*************** With* 注入：在中间件里往 context 写入 ***************/

func WithTenantID(ctx context.Context, tenantID uint64) context.Context {
	return context.WithValue(ctx, TenantIDKey, tenantID)
}

func WithTenantUUID(ctx context.Context, tenantUUID string) context.Context {
	return context.WithValue(ctx, TenantUUIDKey, tenantUUID)
}

func WithSubject(ctx context.Context, subject string) context.Context {
	return context.WithValue(ctx, SubjectKey, subject)
}

func WithScope(ctx context.Context, scope string) context.Context {
	return context.WithValue(ctx, ScopeKey, scope)
}

func WithAudience(ctx context.Context, aud string) context.Context {
	return context.WithValue(ctx, AudienceKey, aud)
}

func WithPlatform(ctx context.Context, platform string) context.Context {
	return context.WithValue(ctx, PlatformKey, platform)
}

func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDKey, traceID)
}

func WithJWTClaims(ctx context.Context, claims *CoreXClaims) context.Context {
	return context.WithValue(ctx, JWTClaimsKey, claims)
}
