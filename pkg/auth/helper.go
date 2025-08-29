// pkg/auth/helper.go
package auth

import (
	"context"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"strconv"
)

/*************** 常用错误 ***************/
var (
	ErrTenantMissing = errors.New("tenant_id missing")
	ErrClaimsMissing = errors.New("jwt claims missing")
)

/*************** Getters：从 context.Context 读取 ***************/

// GetTenantID 返回租户ID；不存在时返回 0（不建议在必须要租户的接口里用）
func GetTenantID(ctx context.Context) uint64 {
	if id, ok := tryUint64(ctx.Value(TenantIDKey)); ok {
		return id
	}
	// 兜底：从 claims 取
	if c := GetJWTClaims(ctx); c != nil && c.TenantID > 0 {
		return c.TenantID
	}
	return 0
}

// TryTenantID 返回 (*id, true) 或 (nil, false)
func TryTenantID(ctx context.Context) (*uint64, bool) {
	if id, ok := tryUint64(ctx.Value(TenantIDKey)); ok {
		return &id, true
	}
	if c := GetJWTClaims(ctx); c != nil && c.TenantID > 0 {
		return &c.TenantID, true
	}
	return nil, false
}

// RequireTenantID 必须有租户，否则报错
func RequireTenantID(ctx context.Context) (uint64, error) {
	if id, ok := TryTenantID(ctx); ok {
		return *id, nil
	}
	return 0, ErrTenantMissing
}

func GetTenantUUID(ctx context.Context) string {
	if v, ok := ctx.Value(TenantUUIDKey).(string); ok && v != "" {
		return v
	}
	// 兜底：claims
	if c := GetJWTClaims(ctx); c != nil {
		return c.TenantUUID
	}
	return ""
}

func GetTraceID(ctx context.Context) string {
	if v, ok := ctx.Value(TraceIDKey).(string); ok {
		return v
	}
	return ""
}
func GetSubject(ctx context.Context) string {
	if v, ok := ctx.Value(SubjectKey).(string); ok {
		return v
	}
	return ""
}
func GetScope(ctx context.Context) string {
	if v, ok := ctx.Value(ScopeKey).(string); ok {
		return v
	}
	return ""
}
func GetAudience(ctx context.Context) string {
	if v, ok := ctx.Value(AudienceKey).(string); ok {
		return v
	}
	return ""
}
func GetPlatform(ctx context.Context) string {
	if v, ok := ctx.Value(PlatformKey).(string); ok {
		return v
	}
	return ""
}

func GetJWTClaims(ctx context.Context) *CoreXClaims {
	if v, ok := ctx.Value(JWTClaimsKey).(*CoreXClaims); ok && v != nil {
		return v
	}
	return nil
}

// 从 claims 派生字段
func GetUserID(ctx context.Context) uint64 {
	if c := GetJWTClaims(ctx); c != nil {
		return c.UserID
	}
	return 0
}
func GetMemberID(ctx context.Context) uint64 {
	if c := GetJWTClaims(ctx); c != nil {
		return c.MemberID
	}
	return 0
}

/*************** Gin 便捷版：从 *gin.Context 读取 ***************/

// TenantIDFromGin：优先从 request.Context() 取；其次兼容 c.Get("tenant_id")；最后兜底 query as_tenant_id
func TenantIDFromGin(c *gin.Context) *uint64 {
	// 1) request.Context()（JwtMiddleware 已写入）
	if id, ok := tryUint64(c.Request.Context().Value(TenantIDKey)); ok {
		return &id
	}
	// 2) 兼容：若中间件（或 CopyCtxToGin）有 Set 到 gin.Context
	if v, ok := c.Get(string(TenantIDKey)); ok {
		if id, ok2 := tryUint64(v); ok2 {
			return &id
		}
	}
	// 3) 兜底：JWT claims
	if v := c.Request.Context().Value(JWTClaimsKey); v != nil {
		if cl, ok := v.(*CoreXClaims); ok && cl != nil && cl.TenantID > 0 {
			return &cl.TenantID
		}
	}
	// 4) Root 代理：?as_tenant_id=
	if s := c.Query("as_tenant_id"); s != "" {
		if u, err := strconv.ParseUint(s, 10, 64); err == nil && u > 0 {
			return &u
		}
	}
	return nil
}

// RequireTenantIDFromGin：必须拿到租户，否则返回错误（用于必须带租户的接口）
func RequireTenantIDFromGin(c *gin.Context) (uint64, error) {
	if id := TenantIDFromGin(c); id != nil {
		return *id, nil
	}
	return 0, ErrTenantMissing
}

// --- 内部工具：把各种可能值转成 uint64（拷贝到本文件，避免依赖别处） ---

func tryUint64(v any) (uint64, bool) {
	switch t := v.(type) {
	case uint64:
		return t, true
	case *uint64:
		if t != nil {
			return *t, true
		}
	case int64:
		if t >= 0 {
			return uint64(t), true
		}
	case *int64:
		if t != nil && *t >= 0 {
			return uint64(*t), true
		}
	case int:
		if t >= 0 {
			return uint64(t), true
		}
	case *int:
		if t != nil && *t >= 0 {
			return uint64(*t), true
		}
	case string:
		if t == "" {
			return 0, false
		}
		if u, err := strconv.ParseUint(t, 10, 64); err == nil {
			return u, true
		}
	}
	return 0, false
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

/*************** 兼容辅助：把 request.Context 的值双写到 gin.Context ***************/

// CopyCtxToGin 把 request.Context() 中的常用键复制到 gin.Context（兼容老代码用 c.Get(...) 的取法）
// 建议在 JwtMiddleware 注入完 request.Context 后调用一次。
func CopyCtxToGin(c *gin.Context) {
	rc := c.Request.Context()

	if v := rc.Value(TenantIDKey); v != nil {
		c.Set(string(TenantIDKey), v)
	}
	if v := rc.Value(TenantUUIDKey); v != nil {
		c.Set(string(TenantUUIDKey), v)
	}
	if v := rc.Value(SubjectKey); v != nil {
		c.Set(string(SubjectKey), v)
	}
	if v := rc.Value(ScopeKey); v != nil {
		c.Set(string(ScopeKey), v)
	}
	if v := rc.Value(AudienceKey); v != nil {
		c.Set(string(AudienceKey), v)
	}
	if v := rc.Value(PlatformKey); v != nil {
		c.Set(string(PlatformKey), v)
	}
	if v := rc.Value(JWTClaimsKey); v != nil {
		c.Set(string(JWTClaimsKey), v)
	}

	// 常用 id（如果中间件有写的话也双写）
	if v := rc.Value(UserIDKey); v != nil {
		c.Set(string(UserIDKey), v)
	}
	if v := rc.Value(MemberIDKey); v != nil {
		c.Set(string(MemberIDKey), v)
	}
	if v := rc.Value(IsRootKey); v != nil {
		c.Set(string(IsRootKey), v)
	}
}

/*************** 内部工具：类型安全的 uint64 解析 ***************/

// RootOnlyCB 返回一个用于 JwtMiddleware 第 5 个参数的回调：仅允许 is_root=true
func RootOnlyCB() func(ctx context.Context, claims *CoreXClaims) error {
	return func(ctx context.Context, claims *CoreXClaims) error {
		if claims == nil || !claims.IsRoot {
			return fmt.Errorf("root only")
		}
		return nil
	}
}
