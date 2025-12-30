package agentlifecycle

import (
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

// requireTenantUUID ensures the request carries a valid tenant UUID in the context.
func requireTenantUUID(c *gin.Context) (string, bool) {
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少有效租户上下文", err)
		return "", false
	}
	tenantUUID = strings.TrimSpace(tenantUUID)
	if tenantUUID == "" {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少有效租户上下文", nil)
		return "", false
	}
	return tenantUUID, true
}
