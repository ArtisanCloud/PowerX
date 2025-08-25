package system

import (
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/bootstrap"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/gin-gonic/gin"
)

func RegisterAPIRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup, deps *bootstrap.Deps) {
	hUser := NewUserHandler(deps.DB)
	gSys := protectedGroup.Group("/admin/system")
	cfg := config.GetGlobalConfig()
	gSys.Use(auth.JwtMiddleware(
		[]byte(cfg.Auth.JWTSecret),
		cfg.Auth.Issuer,
		[]string{cfg.Auth.AudienceUser},
		[]string{"access"},
		auth.RootOnlyCB(),
	))

	gSysUsers := gSys.Group("/users") // 仅 Root 可访问
	{
		gSysUsers.GET("", hUser.List)
		gSysUsers.GET("/:id", hUser.Get)
		gSysUsers.POST("", hUser.Create)
		gSysUsers.POST("/:id/members", hUser.Create)
		gSysUsers.PATCH("/:id/add-to-tenant", hUser.AddToTenant)
		gSysUsers.PATCH("/:id", hUser.Update)
		gSysUsers.PUT("/:id/status", hUser.SetStatus)
		gSysUsers.DELETE("/:id", hUser.Delete)
		gSysUsers.PUT("/:id/restore", hUser.Restore)
		gSysUsers.POST("/:id/force-logout", hUser.ForceLogout)
	}

}
