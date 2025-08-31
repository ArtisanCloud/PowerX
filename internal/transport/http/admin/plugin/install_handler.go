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

	mgr := mgrimpl.GetPluginManager() // 你走“实现包全局”
	p, err := mgr.InstallFromFile(c, req.SrcDir, plugin_mgr.InstallOptions{
		// 先借用 VerifyChecksum 作为“安装后启用”开关
		VerifyChecksum: req.Enable,
	})
	if err != nil {
		dtoRequest.ResponseError(c, plugin_mgr.HTTPStatusOf(plugin_mgr.CodeOf(err)), "安装失败", err)
		return
	}
	// 安装接口返回“刚安装的版本”（已在 InstallFromFile 内保证）
	dtoRequest.ResponseSuccess(c, gin.H{
		"installed": gin.H{
			"id":      p.ID,
			"version": p.Version,
			"state":   p.State,
		},
	})
}

// --- Switch Version ---

// POST /api/admin/plugins/:id/switch_version
type switchVersionReq struct {
	Version string `json:"version" binding:"required"`
	Enable  bool   `json:"enable"`
}

func PluginSwitchVersionHandler(c *gin.Context) {
	var req switchVersionReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	id := c.Param("id")
	if id == "" {
		dtoRequest.ResponseError(c, 400, "缺少插件ID", nil)
		return
	}

	mgr := mgrimpl.GetPluginManager() // 走实现包的全局
	p, err := mgr.SwitchVersion(c, id, req.Version, req.Enable)
	if err != nil {
		dtoRequest.ResponseError(c, plugin_mgr.HTTPStatusOf(plugin_mgr.CodeOf(err)), "切换版本失败", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{
		"id":      p.ID,
		"version": p.Version,
		"state":   p.State,
	})
}

// POST /api/admin/plugins/install/url
// body: {"url":"https://.../com.powerx.demo.hello_world-0.1.2.zip","sha256":"...","enable":true}
type installURLReq struct {
	URL    string `json:"url"     validate:"required,url"`
	SHA256 string `json:"sha256"`    // 可选
	Enable bool   `json:"enable"`    // 安装后是否启用并切 current
	Sign   string `json:"signature"` // 可选：预留
}

func PluginInstallURLHandler(c *gin.Context) {
	var req installURLReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	mgr := mgrimpl.GetPluginManager()

	// 安装（只登记、不自动启用）
	p, err := mgr.InstallFromURL(c, req.URL, req.SHA256, req.Sign, plugin_mgr.InstallOptions{
		VerifyChecksum:  req.SHA256 != "", // 传了就校验
		VerifySignature: false,            // 先关；后续接公钥再开
	})
	if err != nil {
		dtoRequest.ResponseError(c, plugin_mgr.HTTPStatusOf(plugin_mgr.CodeOf(err)), "安装失败", err)
		return
	}

	// 可选：安装完切换并启用该版本
	if req.Enable {
		if _, err := mgr.SwitchVersion(c, p.ID, p.Version, true); err != nil {
			dtoRequest.ResponseError(c, plugin_mgr.HTTPStatusOf(plugin_mgr.CodeOf(err)), "安装成功但启用失败", err)
			return
		}
	}

	dtoRequest.ResponseSuccess(c, gin.H{
		"installed": gin.H{
			"id":      p.ID,
			"version": p.Version,
			"state":   string(p.State),
		},
		"enabled": req.Enable,
	})
}
