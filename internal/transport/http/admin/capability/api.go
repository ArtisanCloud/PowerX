package capability

import (
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
)

// RegisterAPIRoutes 挂载能力契约管理相关的 HTTP 路由。
func RegisterAPIRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	cfg := config.GetGlobalConfig()

	contractHandler := NewContractHandler(deps)
	policyHandler := NewVersionPolicyHandler(deps)

	grp := protectedGroup.Group("/admin/capabilities")
	grp.Use(middleware.JwtMiddleware(
		[]byte(cfg.Auth.JWTSecret),
		cfg.Auth.Issuer,
		[]string{cfg.Auth.AudienceUser},
		[]string{"access"},
		reqctx.RootOnlyCB(),
	))
	{
		grp.GET("", contractHandler.ListContracts)
		grp.POST("", contractHandler.CreateContract)

		grp.GET("/:capabilityKey/versions/:version", contractHandler.GetContract)
		grp.PUT("/:capabilityKey/versions/:version", contractHandler.UpdateContract)
		grp.POST("/:capabilityKey/versions/:version/publish", contractHandler.PublishContract)
		grp.POST("/:capabilityKey/versions/:version/deprecate", contractHandler.DeprecateContract)

		grp.GET("/:capabilityKey/version-policy", policyHandler.GetVersionPolicy)
		grp.PUT("/:capabilityKey/version-policy", policyHandler.UpsertVersionPolicy)
	}
}
