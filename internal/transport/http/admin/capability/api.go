package capability

import (
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

// RegisterAPIRoutes 挂载能力契约管理相关的 HTTP 路由。
func RegisterAPIRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup, cfg *config.Config, deps *shared.Deps) {
	handler := NewContractHandler(deps)
	registerRoutes(protectedGroup, cfg, handler)
}
