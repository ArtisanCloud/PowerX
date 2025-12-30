package testenv

import (
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
)

// AgentModelHubTenantUUID 是测试环境默认使用的租户 UUID。
const AgentModelHubTenantUUID = "d1c92f0b-2f5d-4c5f-b5f2-9b0a0c26f34d"

// RequireAgentModelHubAuth 返回注入租户上下文的 gin middleware，复用测试用 JWT 检查。
func RequireAgentModelHubAuth() gin.HandlerFunc {
	return RequireTenantAuth()
}

// RequireTenantAuth 将 Authorization 校验与租户 UUID 注入结合，方便 HTTP 测试。
func RequireTenantAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		tenantUUID := strings.TrimSpace(c.GetHeader("X-Tenant-UUID"))
		if tenantUUID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing X-Tenant-UUID header"})
			return
		}
		ctx := reqctx.WithTenantUUID(c.Request.Context(), tenantUUID)
		reqctx.CopyCtxToGin(c)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
