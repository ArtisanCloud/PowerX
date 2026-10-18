package media

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

// RegisterAPIRoutes 挂载媒体资产路由。
func RegisterAPIRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	if deps == nil || deps.MediaSvc == nil {
		return
	}
	handler := NewHandler(deps.MediaSvc)
	group := protectedGroup.Group("/admin/media/assets")
	{
		group.POST("", handler.CreateAsset)
		group.GET("", handler.ListAssets)
		group.GET("/:uuid", handler.GetAsset)
		group.GET("/:uuid/resource", handler.Resource)
		group.GET("/:uuid/variants/:variant/resource", handler.VariantResource)
		group.PATCH("/:uuid", handler.UpdateAsset)
		group.DELETE("/:uuid", handler.DeleteAsset)
		group.DELETE("/:uuid/purge", handler.PurgeAsset)
		group.POST("/:uuid/presign", handler.PresignAsset)
		group.GET("/:uuid/derivatives", handler.ImageDerivatives)
		group.GET("/:uuid/derivatives/events", func(c *gin.Context) {
			if deps.EventFabric == nil {
				handler.derivativeEvents(c, nil)
				return
			}
			handler.derivativeEvents(c, deps.EventFabric.RedisClient)
		})
		group.POST("/:uuid/derivatives/ensure", handler.EnsureImageDerivatives)
	}
}
