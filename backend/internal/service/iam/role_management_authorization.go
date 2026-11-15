package iam

import (
	"context"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/auth"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"gorm.io/gorm"
)

// Role administration is a user-session operation. JWT role names and plugin
// permissions with the same triple do not confer Core IAM management authority.
func authorizeRoleManagement(ctx context.Context, db *gorm.DB, actor ActorContext, action string) error {
	claims := reqctx.GetClaims(ctx)
	if claims == nil || claims.UserID == 0 || claims.Scope != "access" || claims.PluginID != "" || strings.EqualFold(strings.TrimSpace(claims.Issuer), "powerx-sts") || reqctx.AuthenticatedAPIKeyHash(ctx) != "" {
		return dto.NewUnauthorized("角色管理需要用户登录身份", nil)
	}
	for _, plane := range claims.Platforms {
		if strings.EqualFold(strings.TrimSpace(plane), "api_key") || strings.EqualFold(strings.TrimSpace(plane), "service_actor") {
			return dto.NewUnauthorized("角色管理需要用户登录身份", nil)
		}
	}
	if auth.IsPlatformRootSession(ctx) {
		if !actor.IsRoot {
			return dto.NewForbidden("角色管理身份不一致", nil)
		}
		return nil
	}
	tenant := strings.TrimSpace(reqctx.GetTenantUUID(ctx))
	if actor.IsRoot || tenant == "" || tenant != strings.TrimSpace(actor.TenantUUID) || reqctx.GetMemberID(ctx) == 0 {
		return dto.NewForbidden("角色管理需要当前租户成员身份", nil)
	}
	permissions := repo.NewPermissionRepository(db)
	allowed, err := permissions.MemberHasLivePermissionFromSources(ctx, tenant, reqctx.GetMemberUUID(ctx), reqctx.GetMemberID(ctx), "iam", "role_management", action, []string{"platform_capability"})
	if err != nil {
		return err
	}
	// Existing deployments may retain a genuinely Core-owned legacy grant.
	// A plugin-owned iam.role triple is never accepted or reassigned here.
	if !allowed {
		allowed, err = permissions.MemberHasLivePermissionFromSources(ctx, tenant, reqctx.GetMemberUUID(ctx), reqctx.GetMemberID(ctx), "iam", "role", action, []string{"core", "platform_capability"})
		if err != nil {
			return err
		}
	}
	if !allowed {
		return dto.NewForbidden("缺少底座角色管理权限（corex.iam.role_management:"+action+"）", nil)
	}
	return nil
}

func (s *RBACService) AuthorizeManagement(ctx context.Context, actor ActorContext, action string) error {
	return authorizeRoleManagement(ctx, s.db, actor, action)
}

func requireRoleTenant(actor ActorContext, role *m.Role) error {
	if !actor.IsRoot && (strings.ToLower(strings.TrimSpace(role.Scope)) != "tenant" || strings.TrimSpace(actor.TenantUUID) == "" || strings.TrimSpace(role.TenantUUID) != strings.TrimSpace(actor.TenantUUID)) {
		return dto.NewForbidden("不能管理其他租户或系统角色", nil)
	}
	return nil
}
