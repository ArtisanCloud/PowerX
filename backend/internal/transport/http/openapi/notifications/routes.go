package notifications

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

// RegisterTenantRoutes 注册租户侧通知调试路由。
func RegisterTenantRoutes(protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	if protectedGroup == nil {
		return
	}
	h := newHandler(deps)
	group := protectedGroup.Group("/notifications")
	group.POST("/test", h.pushTest)
}
