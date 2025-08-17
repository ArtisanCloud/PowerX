package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret []byte // 需在bootstrap时从环境变量设定
const X_TRACE_ID = "X-Trace-ID"

// AuthCallback 解析后外部判断/扩展回调函数
type AuthCallback func(ctx context.Context, claims *CoreXClaims) error

// SetJWTSecret 设置JWT密钥
func SetJWTSecret(secret []byte) {
	jwtSecret = secret
}

// JwtMiddleware JWT中间件
func JwtMiddleware(expectedAudience string, requiredScopes []string, callback AuthCallback) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "缺少Bearer令牌"})
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		token, err := jwt.ParseWithClaims(tokenStr, &CoreXClaims{}, func(token *jwt.Token) (interface{}, error) {
			// 验证签名方法
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("意外的签名方法: %v", token.Header["alg"])
			}
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "无效令牌", "detail": err.Error()})
			return
		}

		claims, ok := token.Claims.(*CoreXClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "无效声明"})
			return
		}
		// logger.Info(fmt.Sprintf("token: %+v", claims))

		// 验证受众
		audienceMatch := false
		for _, aud := range claims.Audience {
			if aud == expectedAudience {
				audienceMatch = true
				break
			}
		}
		if !audienceMatch {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "受众不匹配"})
			return
		}

		// 验证权限范围
		for _, req := range requiredScopes {
			if !strings.Contains(claims.Scope, req) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "权限不足", "need": req})
				return
			}
		}

		// 获取追踪ID
		traceID := c.GetHeader(X_TRACE_ID)
		if traceID == "" {
			traceID = claims.ID // 使用jti作为fallback
		}

		// 将信息注入到上下文中
		ctx := context.WithValue(c.Request.Context(), TenantIDKey, claims.TenantID)
		ctx = context.WithValue(ctx, SubjectKey, claims.Subject)
		ctx = context.WithValue(ctx, ScopeKey, claims.Scope)
		ctx = context.WithValue(ctx, AudienceKey, claims.Audience)
		ctx = context.WithValue(ctx, PlatformKey, claims.Platform)
		ctx = context.WithValue(ctx, TraceIDKey, traceID)
		ctx = context.WithValue(ctx, JWTClaimsKey, claims)
		c.Request = c.Request.WithContext(ctx)

		// 执行回调函数
		if callback != nil {
			if err := callback(c.Request.Context(), claims); err != nil {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "回调拒绝", "detail": err.Error()})
				return
			}
		}

		c.Next()
	}
}
