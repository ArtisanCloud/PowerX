package auth

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

func RegisterAPIRoutes(
	publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup,
	deps *shared.Deps,
) {
	hAuthUser := NewAuthUserHandler(deps)
	authPublicGroup := publicGroup.Group("/admin/user/auth")
	{
		authPublicGroup.POST("/register", hAuthUser.RegisterHandler(deps.AuthUser)) // 如不开放注册，可以先注释
		authPublicGroup.POST("/login", hAuthUser.LoginHandler(deps.AuthUser))
		authPublicGroup.POST("/refresh", hAuthUser.RefreshHandler(deps.AuthUser))
	}
	authProtectedAdminGroup := protectedGroup.Group("/admin/user/auth")
	{
		authProtectedAdminGroup.POST("/logout", hAuthUser.LogoutHandler(deps.AuthUser))
	}

	hMeContext := NewMeContextHandler(deps)

	// 统一用户上下文路由：/api/v1/admin/user/auth/me/*
	gMeContext := protectedGroup.Group("/admin/user/auth")
	{
		gMeContext.GET("/me/context", hMeContext.GetMeContext)
		hMeExtra := NewMeExtraHandler(deps)
		gMeContext.POST("/me/switch-tenant", hMeExtra.SwitchTenant)
		gMeContext.GET("/me/tenants", hMeExtra.ListTenants)
		gMeContext.GET("/me/departments", hMeExtra.ListDepartments)
	}
}
