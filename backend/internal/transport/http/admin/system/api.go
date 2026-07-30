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
	hSetup := NewSetupHandler(deps.DB)
	publicGroup.GET("/admin/setup/status", hSetup.Status)
	publicGroup.GET("/admin/setup/config", hSetup.GetConfig)
	publicGroup.PUT("/admin/setup/config", hSetup.SaveConfig)
	publicGroup.POST("/admin/setup/provision", hSetup.Provision)
	publicGroup.POST("/admin/setup/complete", hSetup.Complete)
	publicGroup.POST("/admin/setup/test/database", hSetup.TestDatabaseConnection)
	publicGroup.POST("/admin/setup/test/cache", hSetup.TestCacheConnection)
	publicGroup.POST("/admin/setup/llm/test-connection", hSetup.TestLLMConnection)
	publicGroup.POST("/admin/setup/llm/test-call", hSetup.TestLLMQuickCall)

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
		gSysUsers.GET("/:user_uuid", hUser.Get)
		gSysUsers.POST("", hUser.Create)
		gSysUsers.PATCH("/:user_uuid/add-to-tenant", hUser.AddToTenant)
		gSysUsers.PATCH("/:user_uuid", hUser.Update)
		gSysUsers.PUT("/:user_uuid/status", hUser.SetStatus)
		gSysUsers.PUT("/:user_uuid/password", hUser.ResetPassword)
		gSysUsers.GET("/:user_uuid/roles", hUser.ListRoles)
		gSysUsers.PUT("/:user_uuid/roles", hUser.SetRoles)
		gSysUsers.DELETE("/:user_uuid", hUser.Delete)
		gSysUsers.PUT("/:user_uuid/restore", hUser.Restore)
		gSysUsers.POST("/:user_uuid/force-logout", hUser.ForceLogout)
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
