package capability_registry

import (
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func requireTenantUUIDParam(c *gin.Context, param string) (string, bool) {
	uuid := strings.TrimSpace(c.Param(param))
	if uuid == "" {
		dto.ResponseError(c, http.StatusBadRequest, "tenant_uuid missing", nil)
		return "", false
	}
	canonical, err := reqctx.CanonicalTenantUUID(uuid)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "tenant_uuid invalid", err)
		return "", false
	}
	return canonical, true
}

func trimTenantUUID(value string) string {
	return strings.TrimSpace(value)
}
