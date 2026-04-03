package migration

import (
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func RequireOpsPermission(deps *shared.Deps, resource, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps == nil || deps.DB == nil {
			dto.ResponseError(c, http.StatusInternalServerError, "ops authorization unavailable", nil)
			c.Abort()
			return
		}

		ctx := c.Request.Context()
		actor := iamsvc.ActorContext{IsRoot: reqctx.IsRoot(ctx), TenantUUID: strings.TrimSpace(reqctx.GetTenantUUID(ctx))}
		if actor.IsRoot {
			c.Next()
			return
		}

		memberID := reqctx.GetMemberID(ctx)
		if memberID == 0 || actor.TenantUUID == "" {
			dto.ResponseError(c, http.StatusUnauthorized, "missing tenant/member context", nil)
			c.Abort()
			return
		}

		svc := iamsvc.NewRBACService(deps.DB)
		pass, err := svc.Enforce(ctx, actor, actor.TenantUUID, memberID, iamsvc.OpsPermissionModule, resource, action)
		if err != nil {
			dto.ResponseError(c, http.StatusForbidden, "rbac check failed", err)
			c.Abort()
			return
		}
		if !pass {
			dto.ResponseError(c, http.StatusForbidden, "forbidden: insufficient ops permission", nil)
			c.Abort()
			return
		}
		c.Next()
	}
}
