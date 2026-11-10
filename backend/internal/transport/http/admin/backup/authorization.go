package backup

import (
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
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

// RequireBackupRoot 限制整机／全库运维，只接受经 JWT 中间件验证的平台 Root 用户。
// 租户 RBAC 和服务身份不具备宿主机配置权；旧路径必须复用相同入口。
func RequireBackupRoot() gin.HandlerFunc {
	return requirePlatformRootSession("BACKUP_ROOT_REQUIRED", "整机备份管理仅允许平台 Root 用户会话")
}

func requirePlatformRootSession(code, message string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		claims := reqctx.GetClaims(c.Request.Context())
		if claims == nil || claims.UserID == 0 {
			dto.RespondErrorFrom(c, dto.NewErrorWithCode(http.StatusUnauthorized, "ROOT_USER_SESSION_REQUIRED", "请使用平台 Root 用户登录", nil))
			c.Abort()
			return
		}
		if !auth.IsPlatformRootSession(c.Request.Context()) {
			dto.RespondErrorFrom(c, dto.NewErrorWithCode(http.StatusForbidden, code, message, nil))
			c.Abort()
			return
		}
		c.Next()
	}
}
