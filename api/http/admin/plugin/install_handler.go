package plugin

import (
	mgrimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
)

// POST /api/admin/plugins/install/local
// body: {"src_dir": "/absolute/or/relative/path", "enable": false}
type installLocalReq struct {
	SrcDir string `json:"src_dir" binding:"required"`
	Enable bool   `json:"enable"`
}

func PluginInstallLocalHandler(c *gin.Context) {
	var req installLocalReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}

	mgr := mgrimpl.GetPluginManager() // ★ 你走实现包的全局
	opts := plugin_mgr.InstallOptions{
		// 临时用 VerifyChecksum 作为“安装后启用”开关
		VerifyChecksum: req.Enable,
	}
	p, err := mgr.InstallFromFile(c, req.SrcDir, opts)
	if err != nil {
		dtoRequest.ResponseError(c, plugin_mgr.HTTPStatusOf(plugin_mgr.CodeOf(err)), "安装失败", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{
		"installed": gin.H{
			"id":      p.ID,
			"version": p.Version,
			"state":   p.State,
		},
	})
}
