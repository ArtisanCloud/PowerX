package capability

import (
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
)

const capabilityContractRoute = "/admin/capability-contracts"

// RegisterAPIRoutes 挂载能力契约管理相关的 HTTP 路由。
func RegisterAPIRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	cfg := config.GetGlobalConfig()

	contractHandler := NewContractHandler(deps)
	policyHandler := NewVersionPolicyHandler(deps)
	adapterHandler := NewAdapterHandler(deps)

	grp := protectedGroup.Group(capabilityContractRoute)
	grp.Use(middleware.JwtMiddleware(
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
		grp.POST("", contractHandler.CreateContract)

		grp.GET("/:capabilityKey/versions/:version", contractHandler.GetContract)
		grp.PUT("/:capabilityKey/versions/:version", contractHandler.UpdateContract)
		grp.POST("/:capabilityKey/versions/:version/publish", contractHandler.PublishContract)
		grp.POST("/:capabilityKey/versions/:version/deprecate", contractHandler.DeprecateContract)

		grp.GET("/:capabilityKey/versions/:version/transports", adapterHandler.ListTransportProfiles)
		grp.PUT("/:capabilityKey/versions/:version/transports", adapterHandler.UpsertTransportProfiles)
		grp.POST("/:capabilityKey/versions/:version/transports/:transport/health", adapterHandler.RunHealthCheck)

		grp.GET("/:capabilityKey/version-policy", policyHandler.GetVersionPolicy)
		grp.PUT("/:capabilityKey/version-policy", policyHandler.UpsertVersionPolicy)
	}
}
