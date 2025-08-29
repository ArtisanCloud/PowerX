// pkg/auth/middleware.go
package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/cache"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/gin-gonic/gin"
)

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

	UserIDKey   ctxKey = "auth.user_id"   // uint64
	MemberIDKey ctxKey = "auth.member_id" // uint64
	IsRootKey   ctxKey = "auth.is_root"   // bool
)

func KUser(uid uint64) string    { return "auth:user:" + strconv.FormatUint(uid, 10) }
func KMember(mid uint64) string  { return "auth:member:" + strconv.FormatUint(mid, 10) }
func KTenant(tid uint64) string  { return "auth:tenant:" + strconv.FormatUint(tid, 10) }
func KRevoked(jti string) string { return "auth:revoked:" + jti }

// JwtMiddleware 统一的 JWT 校验中间件（v5 版）
// - issuer/audiences：与签发保持一致（从配置传入）
// - requiredScopes：允许的 scope（例如只允许 "access"），传空则不限制；支持 "*" 代表任意
// - cb：通过则回调做额外校验（比如租户冻结、风控等）；返回 error 即拒绝
func JwtMiddleware(
	secret []byte,
	issuer string,
	audiences []string,
	requiredScopes []string,
	cb func(ctx context.Context, claims *CoreXClaims) error,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1) Authorization: Bearer <token>
		authz := c.GetHeader("Authorization")
		if authz == "" || !strings.HasPrefix(strings.ToLower(authz), "bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid Authorization header"})
			return
		}
		tokenString := strings.TrimSpace(authz[len("Bearer "):])
		if tokenString == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid bearer token"})
			return
		}

		reqCtx := c.Request.Context()

		// A. 解析 + 标准校验（Issuer / Audience / exp / nbf / iat / 签名）
		claims, err := ParseAndValidate(tokenString, secret, issuer, audiences...)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		// B. scope 检查
		if len(requiredScopes) > 0 && !scopeAllowed(claims.Scope, requiredScopes) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "scope not allowed"})
			return
		}

		// C. 撤销/强制下线（仅 access 携带 jti 时检查）
		authCache := cache.GetCache()
		if authCache != nil && claims.ID != "" {
			if ok, _ := authCache.Exists(reqCtx, KRevoked(claims.ID)); ok {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token revoked"})
				return
			}
		}

		// D. Root 代理租户（仅 is_root=true 生效）
		tid := claims.TenantID
		if claims.IsRoot {
			if as := c.Query("as_tenant_id"); as != "" {
				if v, err := strconv.ParseUint(as, 10, 64); err == nil && v > 0 {
					tid = v
				}
			}
		}

		// E. 读取缓存快照（命中才做轻量校验；未命中交给 cb 处理）
		var userSnap, memberSnap, tenantSnap map[string]any
		if authCache != nil {
			if b, _ := authCache.Get(reqCtx, KUser(claims.UserID)); len(b) > 0 {
				_ = json.Unmarshal(b, &userSnap)
			}
			// 非 Root 才尝试读 member 快照；Root 代理时不需要 member 校验
			if !claims.IsRoot && claims.MemberID > 0 {
				if b, _ := authCache.Get(reqCtx, KMember(claims.MemberID)); len(b) > 0 {
					_ = json.Unmarshal(b, &memberSnap)
				}
			}
			if b, _ := authCache.Get(reqCtx, KTenant(tid)); len(b) > 0 {
				_ = json.Unmarshal(b, &tenantSnap)
			}
		}

		// F. 轻量状态校验（仅在命中缓存时执行）
		if userSnap != nil {
			if st, ok := utils.AsInt16(userSnap["status"]); ok && st != 1 {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "user disabled"})
				return
			}
		}
		// Root 场景不校验 member（因为可能没有与 as_tenant_id 对应的 member）
		if !claims.IsRoot && memberSnap != nil {
			if st, ok := utils.AsInt16(memberSnap["status"]); ok && st != 1 {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "member disabled"})
				return
			}
			if mtid, ok := utils.AsUint64(memberSnap["tenant_id"]); ok && mtid != tid {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "tenant mismatch"})
				return
			}
			if uid, ok := utils.AsUint64(memberSnap["user_id"]); ok && uid != claims.UserID {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "user mismatch"})
				return
			}
		}
		if tenantSnap != nil {
			if st, ok := utils.AsInt16(tenantSnap["status"]); ok && st != 1 {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "tenant disabled"})
				return
			}
		}

		// G. 注入上下文（保留原键；附加常用 id / is_root 和快照）
		reqCtx = context.WithValue(reqCtx, TenantIDKey, tid)
		reqCtx = context.WithValue(reqCtx, TenantUUIDKey, claims.TenantUUID)
		reqCtx = context.WithValue(reqCtx, SubjectKey, claims.MemberUUID) // sub = member.uuid
		reqCtx = context.WithValue(reqCtx, ScopeKey, claims.Scope)
		if len(claims.Audience) > 0 {
			reqCtx = context.WithValue(reqCtx, AudienceKey, claims.Audience[0])
		}
		reqCtx = context.WithValue(reqCtx, PlatformKey, claims.Platforms)
		reqCtx = context.WithValue(reqCtx, JWTClaimsKey, claims)

		// 常用 id / root
		reqCtx = context.WithValue(reqCtx, UserIDKey, claims.UserID)
		reqCtx = context.WithValue(reqCtx, MemberIDKey, claims.MemberID)
		reqCtx = context.WithValue(reqCtx, IsRootKey, claims.IsRoot)

		// 快照（map[string]any）
		if userSnap != nil {
			reqCtx = context.WithValue(reqCtx, "auth.user.snapshot", userSnap)
		}
		if memberSnap != nil {
			reqCtx = context.WithValue(reqCtx, "auth.member.snapshot", memberSnap)
		}
		if tenantSnap != nil {
			reqCtx = context.WithValue(reqCtx, "auth.tenant.snapshot", tenantSnap)
		}
		c.Request = c.Request.WithContext(reqCtx)
		CopyCtxToGin(c)

		// H. 业务回调：缓存 miss 或需要强校验时，cb 回源 DB
		if cb != nil {
			if err := cb(reqCtx, claims); err != nil {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": err.Error()})
				return
			}
		}

		c.Next()
	}
}

func scopeAllowed(got string, allow []string) bool {
	if got == "" {
		return false
	}
	for _, a := range allow {
		if a == "*" || a == got {
			return true
		}
	}
	return false
}
