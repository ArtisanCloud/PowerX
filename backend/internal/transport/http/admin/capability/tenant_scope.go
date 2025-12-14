package capability

import (
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func requireTenantUUID(c *gin.Context) (string, bool) {
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil || strings.TrimSpace(tenantUUID) == "" {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少有效租户上下文", err)
		return "", false
	}
	return strings.TrimSpace(tenantUUID), true
}
