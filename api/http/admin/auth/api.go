package auth

import (
	authsvc "github.com/ArtisanCloud/PowerX/pkg/corex/iam/service"
	"github.com/gin-gonic/gin"
)

func RegisterAPIRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup, s *authsvc.AuthService) {
	authPublicGroup := publicGroup.Group("/user/auth")
	{
		authPublicGroup.POST("/register", RegisterHandler(s)) // 如不开放注册，可以先注释
		authPublicGroup.POST("/login", LoginHandler(s))
		authPublicGroup.POST("/refresh", RefreshHandler(s))
	}

	authProtectedGroup := protectedGroup.Group("/user/auth")
	{
		authProtectedGroup.POST("/logout", LogoutHandler(s))
	}
}
