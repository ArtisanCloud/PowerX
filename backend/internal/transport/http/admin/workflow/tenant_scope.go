package workflow

import (
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

// requireTenantContext 校验租户上下文并返回规范化 UUID。
func requireTenantContext(c *gin.Context) (string, bool) {
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil || strings.TrimSpace(tenantUUID) == "" {
		dto.RespondErrorFrom(c, dto.NewUnauthorized("tenant context missing", err))
		return "", false
	}
	return strings.TrimSpace(strings.ToLower(tenantUUID)), true
}
