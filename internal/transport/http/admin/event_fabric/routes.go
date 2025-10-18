package event_fabric

import (
	"net/http"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

// RegisterAPIRoutes 先行占位事件骨干相关的 Admin API 路由。
// 后续任务会逐步挂载实际 Handler，在此之前提供明确的 501 响应，便于外部感知尚未实现。
func RegisterAPIRoutes(_ *gin.RouterGroup, protected *gin.RouterGroup, _ *shared.Deps) {
	eventGroup := protected.Group("/event-fabric")
	eventGroup.GET("/status", func(c *gin.Context) {
		dto.RespondErrorFrom(c, dto.NewError(http.StatusNotImplemented, "Event Fabric Admin API 尚未实现", nil))
	})
}
