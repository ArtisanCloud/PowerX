package plugin

import (
	"github.com/ArtisanCloud/PowerX/config"
	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
)

// --- Uninstall ---
// POST /api/admin/plugins/:id/uninstall
// body: {"version":"0.1.1","purge":true}
type uninstallReq struct {
	Version string `json:"version"` // 为空则卸载 current
	Purge   bool   `json:"purge"`   // 是否连同磁盘产物清理
}

func PluginUninstallHandler(c *gin.Context) {
	var req uninstallReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	id := c.Param("id")
	if id == "" {
		dtoRequest.ResponseError(c, 400, "缺少插件ID", nil)
		return
	}
	if req.Purge {
		cfg := config.GetGlobalConfig()
		if cfg == nil || !cfg.Plugin.AllowDestructiveDBCleanup {
			dtoRequest.ResponseError(c, 403, "卸载失败", plugin_mgr.NewError(
				plugin_mgr.CodeForbidden,
				plugin_mgr.WithOp("uninstall"),
				plugin_mgr.WithMsg("purge disabled: set plugin.allow_destructive_db_cleanup=true to enable"),
			))
			return
		}
	}

	mgr, err := tryGetPluginManager()
	ctx := c.Request.Context()
	if err != nil {
		if fallbackErr := uninstallFromRegistry(ctx, id, req.Version, req.Purge); fallbackErr != nil {
			respondPluginRuntimeUnavailable(c, fallbackErr)
			return
		}
		dtoRequest.ResponseSuccess(c, gin.H{
			"id":       id,
			"version":  req.Version,
			"purge":    req.Purge,
			"fallback": true,
		})
		return
	}

	if req.Purge {
		if req.Version != "" {
			err = mgr.UninstallAndPurge(ctx, id, req.Version)
		} else {
			err = mgr.UninstallAndPurge(ctx, id)
		}
	} else {
		if req.Version != "" {
			err = mgr.Uninstall(ctx, id, req.Version)
		} else {
			err = mgr.Uninstall(ctx, id)
		}
	}
	if err != nil {
		dtoRequest.ResponseError(c, plugin_mgr.HTTPStatusOf(plugin_mgr.CodeOf(err)), "卸载失败", err)
		return
	}

	dtoRequest.ResponseSuccess(c, gin.H{
		"id":      id,
		"version": req.Version, // 为空=卸载当时的 current
		"purge":   req.Purge,
	})
}
