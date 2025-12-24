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
		group.PATCH("/:uuid", handler.UpdateAsset)
		group.DELETE("/:uuid", handler.DeleteAsset)
		group.POST("/:uuid/presign", handler.PresignAsset)
	}
}
