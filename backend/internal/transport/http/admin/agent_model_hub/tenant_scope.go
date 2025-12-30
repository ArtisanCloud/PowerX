package agentmodelhub

import (
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func requireTenantUUID(c *gin.Context) (string, bool) {
	if c == nil {
		return "", false
	}
	uuid, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少租户上下文", err)
		return "", false
	}
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少租户上下文", nil)
		return "", false
	}
	return uuid, true
}
