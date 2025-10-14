package capability

import (
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
)

func registerRoutes(group *gin.RouterGroup, cfg *config.Config, h *ContractHandler) {
	grp := group.Group("/admin/capabilities")
	grp.Use(middleware.JwtMiddleware(
		[]byte(cfg.Auth.JWTSecret),
		cfg.Auth.Issuer,
		[]string{cfg.Auth.AudienceUser},
		[]string{"access"},
		reqctx.RootOnlyCB(),
	))
	{
		grp.GET("", h.ListContracts)
		grp.POST("", h.CreateContract)

		grp.GET("/:capabilityKey/versions/:version", h.GetContract)
		grp.PUT("/:capabilityKey/versions/:version", h.UpdateContract)
		grp.POST("/:capabilityKey/versions/:version:publish", h.PublishContract)
		grp.POST("/:capabilityKey/versions/:version:deprecate", h.DeprecateContract)
	}
}
