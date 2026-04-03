package ops

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

// RegisterAPIRoutes 预留 Ops 管理域路由组挂载点（当前仅建组，不注册具体 handler）。
func RegisterAPIRoutes(_ *gin.RouterGroup, protectedGroup *gin.RouterGroup, _ *shared.Deps) {
	if protectedGroup == nil {
		return
	}

	opsGroup := protectedGroup.Group("/admin/ops")
	deployGroup := protectedGroup.Group("/admin/deploy")
	pluginGroup := protectedGroup.Group("/admin/plugins")
	backupGroup := protectedGroup.Group("/admin/backup")
	migrationGroup := protectedGroup.Group("/admin/migration")

	_ = opsGroup
	_ = deployGroup
	_ = pluginGroup
	_ = backupGroup
	_ = migrationGroup
}
