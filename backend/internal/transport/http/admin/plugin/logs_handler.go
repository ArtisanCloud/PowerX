package plugin

import (
	mgrimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"strconv"
)

type logsReq struct {
	Bytes int `form:"bytes"` // 拉取尾部 N 字节，默认 16384
}

func PluginLogsHandler(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		dtoRequest.ResponseError(c, 400, "缺少插件ID", nil)
		return
	}
	// 解析 ?bytes=
	tail := 16384
	if v := c.Query("bytes"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			tail = n
		}
	}

	mgr := mgrimpl.GetPluginManager()

	type logsProvider interface {
		RuntimeLogs(id string, tailBytes int) (string, bool)
	}
	content := "" // ⛳ 默认空串，避免 null
	if lp, ok := mgr.(logsProvider); ok {
		if s, ok := lp.RuntimeLogs(id, tail); ok && s != "" {
			content = s
		}
	}

	dtoRequest.ResponseSuccess(c, gin.H{
		"id":         id,
		"tail_bytes": tail,
		"content":    content, // ⛳ 保证是 string，不会是 null
	})
}
