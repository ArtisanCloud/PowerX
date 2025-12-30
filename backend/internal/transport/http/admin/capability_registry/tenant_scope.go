package capability_registry

import (
	"strings"

	capability_registrydto "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/capability_registry/dto"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
)

func requireTenantUUIDParam(c *gin.Context, param string) (string, bool) {
	uuid := strings.TrimSpace(c.Param(param))
	if uuid == "" {
		capability_registrydto.RespondError(c, capability_registrydto.ErrTenantUUIDMissing, nil)
		return "", false
	}
	canonical, err := reqctx.CanonicalTenantUUID(uuid)
	if err != nil {
		capability_registrydto.RespondError(c, capability_registrydto.ErrTenantUUIDInvalid, err)
		return "", false
	}
	return canonical, true
}

func trimTenantUUID(value string) string {
	return strings.TrimSpace(value)
}
