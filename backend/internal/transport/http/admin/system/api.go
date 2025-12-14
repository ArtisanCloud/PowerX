package system

// internal/transport/http/admin/system/api.go
import (
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
)

func RegisterAPIRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	hUser := NewUserHandler(deps)
	gSys := protectedGroup.Group("/admin/system")
	cfg := config.GetGlobalConfig()
	gSys.Use(middleware.JwtMiddleware(
		[]byte(cfg.Auth.JWTSecret),
		cfg.Auth.Issuer,
		[]string{cfg.Auth.AudienceUser},
		[]string{"access"},
		reqctx.RootOnlyCB(),
		middleware.WithTenantHeaderPolicy(middleware.TenantHeaderPolicy{
			RequireUUID: cfg.Tenants.RequireUUID,
		}),
	))

	hSTS := NewSTSHandler(cfg)
	gSys.POST("/sts/:pluginId", hSTS.Mint)

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

	h := NewSettingHandler(deps.DB)

	g := gSys.Group("/settings")
	{
		// 系统级 KV
		g.GET("/system", h.ListSystem) // ?group=&prefix=&page=&pageSize=
		g.GET("/system/:key", h.GetSystem)
		g.PUT("/system/:key", h.UpsertSystem)    // body: { value_json, group?, description?, editable? }
		g.DELETE("/system/:key", h.DeleteSystem) // body: { soft?: true }

		// 租户级 KV（上下文租户，不再依赖路径参数）
		g.GET("/tenant", h.ListTenant) // ?prefix=&page=&pageSize=
		g.GET("/tenant/:key", h.GetTenant)
		g.PUT("/tenant/:key", h.UpsertTenant) // body 同上
		g.DELETE("/tenant/:key", h.DeleteTenant)

		// 生效值（仅 DB 层：tenant > system）
		g.GET("/effective/:key", h.GetEffective) // ?tenant_uuid=（可选，默认使用上下文）
	}

}
