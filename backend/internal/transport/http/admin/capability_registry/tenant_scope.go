package capability_registry

import (
	"strings"

	caperrdto "github.com/ArtisanCloud/PowerX/internal/dto/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
)

func requireTenantUUIDParam(c *gin.Context, param string) (string, bool) {
	uuid := strings.TrimSpace(c.Param(param))
	if uuid == "" {
		caperrdto.RespondError(c, caperrdto.ErrTenantUUIDMissing, nil)
		return "", false
	}
	canonical, err := reqctx.CanonicalTenantUUID(uuid)
	if err != nil {
		caperrdto.RespondError(c, caperrdto.ErrTenantUUIDInvalid, err)
		return "", false
	}
	return canonical, true
}

func trimTenantUUID(value string) string {
	return strings.TrimSpace(value)
}
