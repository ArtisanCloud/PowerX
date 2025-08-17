package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestGenerateAndParseJWT(t *testing.T) {
	// 设置测试密钥
	secret := []byte("test-secret-key")
	SetJWTSecret(secret)

	// 生成JWT令牌
	tenantID := "t-test"
	subject := "user123"
	platform := "web"
	audience := "corex-api"
	scope := "read write"
	ttl := time.Hour

	token, err := GenerateJWT(tenantID, subject, platform, audience, scope, ttl, secret)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	// 设置Gin为测试模式
	gin.SetMode(gin.TestMode)

	// 创建测试路由
	r := gin.New()
	r.GET("/auth/test", JwtMiddleware(audience, []string{"read"}, nil), func(c *gin.Context) {
		// 验证上下文中的值
		assert.Equal(t, tenantID, GetTenantID(c.Request.Context()))
		assert.Equal(t, subject, GetSubject(c.Request.Context()))
		assert.Equal(t, platform, GetPlatform(c.Request.Context()))
		assert.Equal(t, scope, GetScope(c.Request.Context()))

		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// 创建测试请求
	req, _ := http.NewRequest("GET", "/auth/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	// 执行请求
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestJWTMiddlewareWithoutToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.GET("/auth/test", JwtMiddleware("corex-api", []string{"read"}, nil), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/auth/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestJWTMiddlewareWithInvalidToken(t *testing.T) {
	secret := []byte("test-secret-key")
	SetJWTSecret(secret)

	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.GET("/auth/test", JwtMiddleware("corex-api", []string{"read"}, nil), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/auth/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestJWTMiddlewareWithInsufficientScope(t *testing.T) {
	secret := []byte("test-secret-key")
	SetJWTSecret(secret)

	// 生成只有read权限的令牌
	token, err := GenerateJWT("t-test", "user123", "web", "corex-api", "read", time.Hour, secret)
	assert.NoError(t, err)

	gin.SetMode(gin.TestMode)

	// 要求write权限
	r := gin.New()
	r.GET("/auth/test", JwtMiddleware("corex-api", []string{"write"}, nil), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/auth/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestJWTMiddlewareWithCallback(t *testing.T) {
	secret := []byte("test-secret-key")
	SetJWTSecret(secret)

	token, err := GenerateJWT("t-test", "user123", "web", "corex-api", "read", time.Hour, secret)
	assert.NoError(t, err)

	gin.SetMode(gin.TestMode)

	// 测试回调函数拒绝访问
	callbackReject := func(ctx context.Context, claims *CoreXClaims) error {
		return fmt.Errorf("回调拒绝访问")
	}

	r := gin.New()
	r.GET("/auth/test", JwtMiddleware("corex-api", []string{"read"}, callbackReject), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/auth/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestBlacklist(t *testing.T) {
	jti := "test-jti-123"

	// 初始状态不在黑名单中
	assert.False(t, IsRevoked(jti))

	// 添加到黑名单
	Revoke(jti, time.Minute)
	assert.True(t, IsRevoked(jti))

	// 测试过期清理
	Revoke("expired-jti", time.Millisecond)
	time.Sleep(time.Millisecond * 2)
	assert.False(t, IsRevoked("expired-jti"))
}
