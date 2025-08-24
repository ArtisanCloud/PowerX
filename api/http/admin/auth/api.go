package auth

import (
	"github.com/ArtisanCloud/PowerX/internal/bootstrap"
	"github.com/gin-gonic/gin"
)

func RegisterAPIRoutes(
	publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup,
	deps *bootstrap.Deps,
) {
	hAuthUser := NewAuthUserHandler(deps)
	authPublicGroup := publicGroup.Group("/admin/user/auth")
	{
		authPublicGroup.POST("/register", hAuthUser.RegisterHandler(deps.AuthUser)) // 如不开放注册，可以先注释
		authPublicGroup.POST("/login", hAuthUser.LoginHandler(deps.AuthUser))
		authPublicGroup.POST("/refresh", hAuthUser.RefreshHandler(deps.AuthUser))
	}

	authProtectedGroup := protectedGroup.Group("/user/auth")
	{
		authProtectedGroup.POST("/logout", hAuthUser.LogoutHandler(deps.AuthUser))
	}

	hMeContext := NewMeContextHandler(deps)

	// 根据你的中间件实际名称绑定，确保需要登录
	gMeContext := protectedGroup.Group("/admin/auth")
	{
		gMeContext.GET("/me/context", hMeContext.GetMeContext)
	}
}
