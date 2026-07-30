package workflow

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	coreiam "github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

func requireWorkflowTenantAdmin(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		if reqctx.IsRoot(ctx) || hasWorkflowTenantAdminRole(ctx, deps) {
			c.Next()
			return
		}
		dto.ResponseError(c, http.StatusForbidden, "workflow.tenant_admin_required", nil)
		c.Abort()
	}
}

func hasWorkflowTenantAdminRole(ctx context.Context, deps *shared.Deps) bool {
	if deps == nil || deps.DB == nil {
		return false
	}

	tenantUUID := strings.TrimSpace(reqctx.GetTenantUUID(ctx))
	memberID := reqctx.GetMemberID(ctx)
	if tenantUUID == "" || memberID == 0 {
		return false
	}

	roles, err := iamrepo.NewRoleBindingRepository(deps.DB).ListRolesByMember(ctx, tenantUUID, memberID)
	if err != nil {
		logger.WarnF(ctx, "[workflow] tenant role check failed tenant=%s member=%d err=%v", tenantUUID, memberID, err)
		return false
	}
	for _, role := range roles {
		if strings.TrimSpace(role.TenantUUID) != tenantUUID {
			continue
		}
		if strings.TrimSpace(strings.ToLower(role.Scope)) != string(coreiam.RoleScopeTenant) {
			continue
		}
		switch role.Code {
		case coreiam.CodeRoleOwner, coreiam.CodeRoleAdmin:
			return true
		}
	}
	return false
}
