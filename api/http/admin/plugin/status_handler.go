package plugin

import (
	mgrimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

// GET /api/admin/plugins/:id/status
func PluginStatusHandler(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		dtoRequest.ResponseError(c, 400, "缺少插件ID", nil)
		return
	}
	// 从全局 Manager 拿 Supervisor 的视图（通过 manager 提供的轻薄封装）
	mgr := mgrimpl.GetPluginManager()

	// 这里为了不暴露实现细节，我们在 manager 包里做个小封装：
	//   func RuntimeStatus(id string) (any, bool)
	// 下面是直接用实现（如果你已经暴露了），否则见 3.2 小封装。
	type statusProvider interface {
		RuntimeStatus(id string) (any, bool)
	}
	if sp, ok := mgr.(statusProvider); ok {
		if st, ok := sp.RuntimeStatus(id); ok {
			dtoRequest.ResponseSuccess(c, st)
			return
		}
	}
	// 兜底：没有运行
	dtoRequest.ResponseSuccess(c, gin.H{
		"id":      id,
		"state":   "stopped",
		"pid":     0,
		"port":    0,
		"healthy": false,
	})
}
