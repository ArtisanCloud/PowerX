package media

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

// Register mounts tenant-facing Media API routes.
func Register(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	if deps == nil || deps.MediaSvc == nil || protectedGroup == nil {
		return
	}
	h := NewHandler(deps.MediaSvc)
	group := protectedGroup.Group("/media/assets")
	group.POST("", h.CreateAsset)
	group.GET("", h.ListAssets)
	group.GET("/:uuid", h.GetAsset)
	group.GET("/:uuid/resource", h.StreamAssetResource)
	group.PATCH("/:uuid", h.UpdateAsset)
	group.DELETE("/:uuid", h.DeleteAsset)
	group.POST("/:uuid/presign", h.PresignAsset)
	group.POST("/:uuid/variants/:variant", h.CreateAssetVariant)
	group.GET("/:uuid/variants/:variant/resource", h.StreamAssetVariantResource)
	group.POST("/:uuid/variants/:variant/presign", h.PresignAssetVariant)
}

// RegisterPublicResource mounts anonymous resource endpoint at root level (e.g., /media/:uuid/resource).
func RegisterPublicResource(engine *gin.Engine, deps *shared.Deps) {
	if engine == nil || deps == nil || deps.MediaSvc == nil {
		return
	}
	h := NewHandler(deps.MediaSvc)
	engine.GET("/media/:uuid/resource", h.StreamAssetResourcePublic)
}
