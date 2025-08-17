package auth

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// SampleCallback 示例回调函数
func SampleCallback(ctx context.Context, claims *CoreXClaims) error {
	tenant := GetTenantID(ctx)

	// 检查租户是否被禁用
	if tenant == "t-banned" {
		return fmt.Errorf("租户已被禁用")
	}

	// 检查JWT是否在黑名单中
	if claims.ID != "" && IsRevoked(claims.ID) {
		return fmt.Errorf("令牌已被撤销")
	}

	// 发布认证成功事件（需要事件总线支持）
	// 这里暂时注释掉，等事件总线完善后再启用
	/*
		event_bus.Publish("auth_succeeded", map[string]interface{}{
			"tenant_id": tenant,
			"subject":   claims.Subject,
			"platform":  claims.Platform,
			"trace_id":  GetTraceID(ctx),
			"scope":     claims.Scope,
		})
	*/

	return nil
}

// ProtectedEndpoint 受保护的端点示例
func ProtectedEndpoint() gin.HandlerFunc {
	return func(c *gin.Context) {
		tenant := GetTenantID(c.Request.Context())
		subject := GetSubject(c.Request.Context())
		platform := GetPlatform(c.Request.Context())
		scope := GetScope(c.Request.Context())
		traceID := GetTraceID(c.Request.Context())

		c.JSON(http.StatusOK, gin.H{
			"tenant_id": tenant,
			"subject":   subject,
			"platform":  platform,
			"scope":     scope,
			"trace_id":  traceID,
			"message":   "访问成功",
		})
	}
}

// AdminOnlyEndpoint 仅管理员可访问的端点示例
func AdminOnlyEndpoint() gin.HandlerFunc {
	return JwtMiddleware("corex-admin", []string{"admin"}, SampleCallback)
}

// AgentEndpoint 智能体端点示例
func AgentEndpoint() gin.HandlerFunc {
	return JwtMiddleware("corex-agent", []string{"agent"}, SampleCallback)
}

// ReadOnlyEndpoint 只读端点示例
func ReadOnlyEndpoint() gin.HandlerFunc {
	return JwtMiddleware("corex-api", []string{"read"}, SampleCallback)
}

// SetupAuthRoutes 设置认证相关路由的示例
func SetupAuthRoutes(r *gin.Engine) {
	// 公开端点
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 需要认证的API组
	api := r.Group("/api/v1")

	// 管理员端点
	admin := api.Group("/admin")
	admin.Use(AdminOnlyEndpoint())
	admin.GET("/users", ProtectedEndpoint())
	admin.GET("/system", ProtectedEndpoint())

	// 智能体端点
	agent := api.Group("/agent")
	agent.Use(AgentEndpoint())
	agent.POST("/tools/:name", ProtectedEndpoint())
	agent.GET("/flows", ProtectedEndpoint())

	// 只读端点
	readonly := api.Group("/data")
	readonly.Use(ReadOnlyEndpoint())
	readonly.GET("/customers", ProtectedEndpoint())
	readonly.GET("/reports", ProtectedEndpoint())
}
