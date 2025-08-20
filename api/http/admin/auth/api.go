package auth

import (
	authsvc "github.com/ArtisanCloud/PowerX/pkg/corex/iam/service"
	"github.com/gin-gonic/gin"
)

func RegisterAPIRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup, s *authsvc.AuthService) {
	menuGroup := publicGroup.Group("/user/auth")
	{
		menuGroup.POST("/register", RegisterHandler(s)) // 如不开放注册，可以先注释
		menuGroup.POST("/login", LoginHandler(s))
		menuGroup.POST("/refresh", RefreshHandler(s))
		menuGroup.POST("/logout", LogoutHandler(s))
	}
}
