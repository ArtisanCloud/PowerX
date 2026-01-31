// pkg/auth/middleware.go
package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func KUser(uid uint64) string    { return "auth:user:" + strconv.FormatUint(uid, 10) }
func KMember(mid uint64) string  { return "auth:member:" + strconv.FormatUint(mid, 10) }
func KTenant(tid uint64) string  { return "auth:tenant:" + strconv.FormatUint(tid, 10) }
func KRevoked(jti string) string { return "auth:revoked:" + jti }

// JwtMiddleware 统一的 JWT 校验中间件
// - issuer/audiences：与签发保持一致（从配置传入）
// - requiredScopes：允许的 scope（例如只允许 "access"），传空则不限制；支持 "*" 代表任意
// - cb：通过则回调做额外校验（比如租户冻结、风控等）；返回 error 即拒绝
func JwtMiddleware(
	secret []byte,
	issuer string,
	audiences []string,
	requiredScopes []string,
	cb func(ctx context.Context, claims *reqctx.CoreXClaims) error,
	opts ...JwtOption,
) gin.HandlerFunc {
	cfg := jwtMiddlewareConfig{
		headerPolicy: TenantHeaderPolicy{
			RequireUUID: true,
		},
	}
	for _, opt := range opts {
		opt(&cfg)
	}
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
		// 注意：ParseAndValidate 需返回 *reqctx.CoreXClaims（见 pkg/auth/jwt.go）
		claims, err := auth.ParseAndValidate(tokenString, secret, issuer, audiences...)
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
		tenantUUID := strings.TrimSpace(claims.TenantUUID)
		if tenantUUID == "" {
			tenantUUID = strings.TrimSpace(c.GetHeader("X-PowerX-Tenant"))
		}
		if claims.IsRoot {
			if asUUID := strings.TrimSpace(c.Query("as_tenant_uuid")); asUUID != "" {
				tenantUUID = asUUID
			} else if headerUUID := strings.TrimSpace(c.GetHeader("X-PowerX-Tenant")); headerUUID != "" {
				// Root 用户允许通过 Header 选择租户（等价于 as_tenant_uuid），用于 Web 管理台切换上下文。
				tenantUUID = headerUUID
			}
		}

		tenantUUID = strings.TrimSpace(tenantUUID)
		tenantID := claims.TenantID
		var tenantUUIDValue uuid.UUID
		if tenantUUID == "" {
			if cfg.headerPolicy.RequireUUID {
				incTenantHeaderReject()
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "tenant uuid required"})
				return
			}
		} else {
			canonical, err := reqctx.CanonicalTenantUUID(tenantUUID)
			if err != nil {
				incTenantHeaderReject()
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid tenant uuid"})
				return
			}
			tenantUUID = canonical
			if parsed, err := uuid.Parse(canonical); err == nil {
				tenantUUIDValue = parsed
			}
		}

		// E. 读取缓存快照（命中才做轻量校验；未命中交给 cb 处理）
		var userSnap, memberSnap map[string]any
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
		}

		// F. 轻量状态校验（仅在命中缓存时执行）
		if userSnap != nil {
			if st, ok := utils.AsInt16(userSnap["status"]); ok && st != 1 {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "user disabled"})
				return
			}
		}
		// Root 场景不校验 member（代理租户时可能没有成员记录）
		if !claims.IsRoot && memberSnap != nil {
			if st, ok := utils.AsInt16(memberSnap["status"]); ok && st != 1 {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "member disabled"})
				return
			}
			if mtid, ok := utils.AsUint64(memberSnap["tenant_id"]); ok && mtid != tenantID {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "tenant mismatch"})
				return
			}
			if uid, ok := utils.AsUint64(memberSnap["user_id"]); ok && uid != claims.UserID {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "user mismatch"})
				return
			}
		}
		// G. 注入上下文（统一使用 reqctx.With*，并修正 platform 类型）
		reqCtx = reqctx.WithClaims(reqCtx, claims)
		reqCtx = reqctx.WithTenantUUID(reqCtx, tenantUUID)
		if tenantUUIDValue != uuid.Nil {
			reqCtx = reqctx.WithTenantUUIDValue(reqCtx, tenantUUIDValue)
		}

		// 常用 id / root
		reqCtx = reqctx.WithUserID(reqCtx, claims.UserID)
		reqCtx = reqctx.WithMemberID(reqCtx, claims.MemberID)
		reqCtx = reqctx.WithIsRoot(reqCtx, claims.IsRoot)

		// subject / audience / platform
		reqCtx = reqctx.WithSubject(reqCtx, claims.MemberUUID)
		if len(claims.Audience) > 0 {
			reqCtx = reqctx.WithAudience(reqCtx, claims.Audience[0])
		}
		plat := ""
		if len(claims.Platforms) > 0 {
			plat = claims.Platforms[0] // 统一写入 string，避免后续读取类型不匹配
		}
		reqCtx = reqctx.WithPlatform(reqCtx, plat)

		// 环境：把 token 里的 env / envs 也写入（下游可直接 reqctx.GetEnv 使用）
		reqCtx = reqctx.WithEnv(reqCtx, claims.Env)
		reqCtx = reqctx.WithEnvs(reqCtx, claims.Envs)

		// TraceID（可选：从 Header 透传）
		if tr := c.GetHeader("X-Trace-Id"); tr != "" {
			reqCtx = reqctx.WithTraceID(reqCtx, tr)
		}

		// 写回 request，并同步到 gin.Context 的 keys
		c.Request = c.Request.WithContext(reqCtx)
		reqctx.CopyCtxToGin(c)
		// 兼容历史：部分 handler 仍通过 "auth_claims" 键读取 claims
		c.Set("auth_claims", *claims)

		// H. 业务回调：缓存 miss 或需要强校验时，cb 回源 DB
		if cb != nil {
			if err := cb(reqCtx, claims); err != nil {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": err.Error()})
				return
			}
		}

		incTenantUUIDOnlyRequest()
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
