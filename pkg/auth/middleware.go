package auth

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// JwtMiddleware 统一的 JWT 校验中间件（v5 版）
// - issuer/audiences：与签发保持一致（从配置传入）
// - requiredScopes：允许的 scope（例如只允许 "access"），传空则不限制；支持 "*" 代表任意
// - cb：通过则回调做额外校验（比如租户冻结、风控等）；返回 error 即拒绝
func JwtMiddleware(
	secret []byte, // 👈 显式传入密钥
	issuer string, // 例如 "corex-auth"
	audiences []string, // 例如 []string{"admin"}
	requiredScopes []string, // 例如 []string{"access"}；传空不限制；"*" 表示任意
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

		// 2) 解析 + 校验（Issuer / Audience / 过期 / 生效 / 签名）
		reqCtx := c.Request.Context()

		// —— 调试：验签前日志（看 issuer/aud/密钥指纹是否一致）
		// logger.Info(reqCtx, "jwt parse(begin)", "issuer", issuer, "aud", audiences, "secret_fp", secretFP(secret))

		claims, err := ParseAndValidate(tokenString, secret, issuer, audiences...)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		// —— 调试：解析后日志（看 token 内部值）
		// logger.Info(reqCtx, "jwt parsed",
		//   "iss", claims.Issuer, "aud", claims.Audience,
		//   "tid", claims.TenantUUID, "mid", claims.MemberUUID, "scope", claims.Scope, "jti", claims.ID)

		// 3) scope 检查
		if len(requiredScopes) > 0 && !scopeAllowed(claims.Scope, requiredScopes) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "scope not allowed"})
			return
		}

		// 4) 注入到 request.Context
		tenantStr := claims.TenantUUID
		if tenantStr == "" && claims.TenantID != 0 {
			tenantStr = strconv.FormatUint(claims.TenantID, 10)
		}
		reqCtx = context.WithValue(reqCtx, TenantIDKey, tenantStr)
		reqCtx = context.WithValue(reqCtx, SubjectKey, claims.MemberUUID) // sub = member.uuid
		reqCtx = context.WithValue(reqCtx, ScopeKey, claims.Scope)
		if len(claims.Audience) > 0 {
			reqCtx = context.WithValue(reqCtx, AudienceKey, claims.Audience[0])
		}
		reqCtx = context.WithValue(reqCtx, PlatformKey, claims.Platforms) // ✅ 修正：Platform
		reqCtx = context.WithValue(reqCtx, JWTClaimsKey, claims)
		c.Request = c.Request.WithContext(reqCtx)

		// 5) 自定义回调（如租户冻结/风控等）
		if cb != nil {
			if err := cb(reqCtx, claims); err != nil {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": err.Error()})
				return
			}
		}

		// 6) 通过
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
