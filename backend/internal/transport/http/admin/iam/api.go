// api/http/admin/iam/api.go
package iam

import (
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	authsvc "github.com/ArtisanCloud/PowerX/internal/service/auth"
	service "github.com/ArtisanCloud/PowerX/internal/service/iam"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
)

// 依赖注入（你项目已有 Deps 可替换）
func RegisterAPIRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	hDept := NewDepartmentHandler(deps)
	gDept := protectedGroup.Group("/admin/iam/departments")
	{
		gDept.GET("/tree", hDept.Tree)
		gDept.POST("", hDept.Create)
		gDept.PATCH("/:id", hDept.Update)
		gDept.DELETE("/:id", hDept.Delete)
	}

	// 成员 CRUD
	memberSvc := service.NewMemberService(deps.DB)
	hMember := NewMemberHandler(memberSvc)

	// /admin/iam/members（保持不变）
	gMember := protectedGroup.Group("/admin/iam/members")
	{
		gMember.GET("", hMember.List)
		gMember.GET("/:id", hMember.Get)
		gMember.POST("", hMember.Create)
		gMember.PATCH("/:id", hMember.Update)
		gMember.PUT("/:id/status", hMember.SetStatus)
		gMember.DELETE("/:id", hMember.Delete)
		gMember.PUT("/:id/restore", hMember.Restore)
		gMember.PUT("/:id/departments", hMember.PutDepartments)
		gMember.POST("/:id/force-logout", hMember.ForceMemberLogout)
	}

	// 新增：/admin/iam/users 分组（租户内视角：按 user 操作其在当前租户的 member）
	gUser := protectedGroup.Group("/admin/iam/users")
	{
		gUser.GET("/:user_id/member", hMember.GetMemberByUser)  // 查询某个 user 在当前租户的 member
		gUser.GET("/members", hMember.BatchGetMembersByUsers)   // 批量：user_ids 查询在本租户的 members
		gUser.POST("/:user_id/member", hMember.AddExistingUser) // 把已有 user 加入当前租户（创建 member）
	}

	h := NewRoleHandler(deps)
	gRoles := protectedGroup.Group("admin/iam/roles")
	{
		gRoles.POST("", h.Create)
		gRoles.GET("", h.List)
		gRoles.GET(":id", h.Get)
		gRoles.PATCH(":id", h.Update)
		gRoles.DELETE(":id", h.Delete)
	}

	// RBAC
	hRBAC := NewRBACHandler(deps)
	gRBAC := protectedGroup.Group("/admin/iam")
	{
		// 角色 ⇄ 权限
		gRoles.POST("/:id/permissions/grant-ids", hRBAC.GrantByIDs)
		gRoles.POST("/:id/permissions/grant", hRBAC.GrantByTriples)
		gRoles.POST("/:id/permissions/revoke-ids", hRBAC.RevokeByIDs)
		gRoles.GET("/:id/permissions", hRBAC.ListPermsOfRole)

		// 角色 ⇄ 成员（直绑）
		gRoles.POST("/:id/bind/member", hRBAC.BindRoleToMember)
		gRoles.DELETE("/:id/bindings/:binding_id", hRBAC.UnbindRoleFromMember)

		gRoles.GET("/:id/permissions/ids", hRBAC.ListPermIDs)
		gRoles.PUT("/:id/permissions/set-ids", hRBAC.SetPermIDs)

		// 当前登录人鉴权检查。
		gRBAC.GET("/me/check", hRBAC.CheckPermission)
	}

	hMigration := NewMigrationHandler(service.NewIAMMigrationReportService(deps.DB))
	gMigration := protectedGroup.Group("/admin/iam/migration")
	{
		gMigration.GET("/report", hMigration.Report)
		gMigration.POST("/fix-owner", hMigration.FixOwner)
	}

	policySvc := authsvc.NewRegistrationPolicyService(deps.DB)
	hRegistration := NewRegistrationAdminHandler(
		policySvc,
		authsvc.NewInviteCodeService(deps.DB),
		authsvc.NewRegistrationRequestService(deps.DB, authsvc.WithRegistrationRequestPolicy(policySvc)),
	)
	gRegistrationPolicy := protectedGroup.Group("/admin/registration-policy")
	{
		gRegistrationPolicy.GET("", hRegistration.GetPolicy)
		gRegistrationPolicy.GET("/history", hRegistration.ListPolicyHistory)
		gRegistrationPolicy.PUT("", hRegistration.UpdateDraft)
		gRegistrationPolicy.POST("/activate", hRegistration.ActivatePolicy)
	}
	gRegistrationInvites := protectedGroup.Group("/admin/registration-invite-batches")
	{
		gRegistrationInvites.GET("", hRegistration.ListInviteBatches)
		gRegistrationInvites.POST("", hRegistration.CreateInviteBatch)
		gRegistrationInvites.DELETE("", hRegistration.DeleteInviteBatches)
		gRegistrationInvites.GET("/:batch_uuid/codes", hRegistration.ListInviteCodes)
		gRegistrationInvites.POST("/:batch_uuid/codes", hRegistration.GenerateInviteCodes)
		gRegistrationInvites.POST("/:batch_uuid/codes/reset-missing-plain", hRegistration.ResetMissingInviteCodePlaintext)
		gRegistrationInvites.POST("/:batch_uuid/pause", hRegistration.PauseInviteBatch)
		gRegistrationInvites.POST("/:batch_uuid/revoke", hRegistration.RevokeInviteBatch)
	}
	gRegistrationRequests := protectedGroup.Group("/admin/registration-requests")
	{
		gRegistrationRequests.GET("", hRegistration.ListRequests)
		gRegistrationRequests.POST("/:request_uuid/approve", hRegistration.ApproveRequest)
		gRegistrationRequests.POST("/:request_uuid/reject", hRegistration.RejectRequest)
	}

	// permission
	hPerm := NewPermissionHandler(deps)

	gPerm := protectedGroup.Group("/admin/iam/permissions")
	gSysPerm := protectedGroup.Group("/admin/iam/permissions")
	cfg := config.GetGlobalConfig()
	gSysPerm.Use(middleware.JwtMiddleware(
		[]byte(cfg.Auth.JWTSecret),
		cfg.Auth.Issuer,
		[]string{cfg.Auth.AudienceUser},
		[]string{"access"},
		reqctx.RootOnlyCB(),
		middleware.WithTenantHeaderPolicy(middleware.TenantHeaderPolicy{
			RequireUUID: cfg.Tenants.RequireUUID,
		}),
	))
	{
		gPerm.GET("", hPerm.List)
		gPerm.GET("/plugin-catalog", hPerm.PluginCatalog)
		gPerm.GET("catalog", hPerm.Catalog)

		// 系统权限：注册、同步、更新、状态切换
		gSysPerm.POST("register", hPerm.Register)
		gSysPerm.POST("sync", hPerm.Sync)
		gSysPerm.DELETE("/plugin-invalid", hPerm.CleanupInvalidPluginPermissions)
		gSysPerm.PATCH("/:id", hPerm.Update)         // 更新描述 /（可选）状态
		gSysPerm.PUT("/:id/status", hPerm.SetStatus) // 仅状态切换
	}
}
