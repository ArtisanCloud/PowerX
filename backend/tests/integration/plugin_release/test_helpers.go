package pluginreleaseintegration

import (
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
)

// injectAdminClaimsMiddleware simulates an authenticated admin/root request context for admin HTTP routes.
func injectAdminClaimsMiddleware(tenantUUID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		baseCtx := c.Request.Context()
		claims := &reqctx.CoreXClaims{
			TenantUUID: tenantUUID,
			IsRoot:     true,
			Roles:      []string{string(iam.CodeSystemAdmin)},
		}
		ctx := reqctx.WithTenantUUID(baseCtx, tenantUUID)
		ctx = reqctx.WithIsRoot(ctx, true)
		ctx = reqctx.WithClaims(ctx, claims)
		c.Request = c.Request.WithContext(ctx)
		reqctx.CopyCtxToGin(c)
		c.Next()
	}
}
