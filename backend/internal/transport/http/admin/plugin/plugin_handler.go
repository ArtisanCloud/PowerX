package plugin

import (
	"github.com/ArtisanCloud/PowerX/config"
	manager "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	pluginDto "github.com/ArtisanCloud/PowerX/pkg/dto/plugin_mgr"
	pluginMgr "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
	"net/http"
	"sort"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

// 辅助：从全局配置拿 BasePrefix（/_p）
func basePrefix() string {
	if cfg := config.GetGlobalConfig(); cfg != nil && cfg.Plugin.BasePrefix != "" {
		return cfg.Plugin.BasePrefix
	}
	return "/_p"
}

// GET /api/.../admin/plugins
func PluginListHandler(c *gin.Context) {
	mgr := manager.GetPluginManager()

	list, err := mgr.List(c)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusInternalServerError, "获取插件列表失败", err)
		return
	}

	prefix := basePrefix()
	out := make([]pluginDto.PluginItem, 0, len(list))

	for _, p := range list {
		adminURL := ""
		hasAdmin := p.Frontend.Admin.Kind == pluginMgr.FrontendKindStatic && p.Paths.FrontendAdminDir != ""
		if hasAdmin {
			adminURL = prefix + "/" + p.ID + "/admin/"
		}

		menus := make([]pluginDto.PluginMenuItem, 0, len(p.Frontend.Admin.Menus))
		for _, m := range p.Frontend.Admin.Menus {
			title := m.Title
			if m.TitleI18n != nil {
				if def := m.TitleI18n.Default; def != "" {
					title = def
				} else if title == "" {
					title = m.TitleI18n.Key
				}
			}
			menus = append(menus, pluginDto.PluginMenuItem{
				ID:    p.ID,
				Title: title,
				Icon:  m.Icon,
				Order: m.Order,
				URL:   adminURL, // 简化：统一指向根；细分路由可在此扩展
			})
		}
		sort.Slice(menus, func(i, j int) bool { return menus[i].Order < menus[j].Order })

		out = append(out, pluginDto.PluginItem{
			ID:       p.ID,
			Name:     p.ID, // 或使用 p.Manifest.Name（若你对外暴露了）
			Version:  p.Version,
			State:    string(p.State),
			AdminURL: adminURL,
			APIBase:  prefix + "/" + p.ID + "/api",
			HasAdmin: hasAdmin,
			Menus:    menus,
		})
	}

	// 启用优先、再按 ID
	sort.Slice(out, func(i, j int) bool {
		if out[i].State != out[j].State {
			return out[i].State == string(pluginMgr.StateEnabled)
		}
		return out[i].ID < out[j].ID
	})

	dtoRequest.ResponseSuccess(c, gin.H{"plugins": out})
}

// POST /api/.../admin/plugins/:id/enable
func PluginEnableHandler(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "缺少插件ID", nil)
		return
	}
	mgr := manager.GetPluginManager()
	// 从已注入的 JWT 上下文读取 tenant_id 并显式写回（确保后续 PostEnable 能准确取到）
	ctx := c.Request.Context()
	if tid := reqctx.GetTenantID(ctx); tid > 0 {
		ctx = reqctx.WithTenantID(ctx, tid)
	}
	if err := mgr.Enable(ctx, id); err != nil {
		dtoRequest.ResponseError(c, statusFromManagerErr(err), "启用插件失败", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
}

// POST /api/.../admin/plugins/:id/disable
func PluginDisableHandler(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "缺少插件ID", nil)
		return
	}
	mgr := manager.GetPluginManager()
	if err := mgr.Disable(c, id); err != nil {
		dtoRequest.ResponseError(c, statusFromManagerErr(err), "停用插件失败", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
}

// GET /api/.../admin/plugins/menus
func PluginMenusHandler(c *gin.Context) {
	mgr := manager.GetPluginManager()
	list, err := mgr.List(c)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusInternalServerError, "获取插件菜单失败", err)
		return
	}

	prefix := basePrefix()
	agg := make([]pluginDto.PluginMenuItem, 0, 16)

	for _, p := range list {
		if p.State != pluginMgr.StateEnabled {
			continue
		}
		if !(p.Frontend.Admin.Kind == pluginMgr.FrontendKindStatic && p.Paths.FrontendAdminDir != "") {
			continue
		}
		base := prefix + "/" + p.ID + "/admin/"
		for _, m := range p.Frontend.Admin.Menus {
			title := m.Title
			if m.TitleI18n != nil {
				if def := m.TitleI18n.Default; def != "" {
					title = def
				} else if title == "" {
					title = m.TitleI18n.Key
				}
			}
			agg = append(agg, pluginDto.PluginMenuItem{
				ID:    p.ID,
				Title: title,
				Icon:  m.Icon,
				Order: m.Order,
				URL:   base,
			})
		}
	}
	sort.Slice(agg, func(i, j int) bool { return agg[i].Order < agg[j].Order })

	dtoRequest.ResponseSuccess(c, gin.H{"menus": agg})
}

// GET /api/.../admin/plugins/:id
// 返回插件详情（合并注册表信息与运行态）
func PluginGetHandler(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "缺少插件ID", nil)
		return
	}
	mgr := manager.GetPluginManager()

	p, err := mgr.Get(c, id)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusNotFound, "插件不存在", err)
		return
	}

	// 运行态
	proc, _ := manager.TryRuntimeStatus(mgr, id)

	prefix := basePrefix()
	hasAdmin := p.Frontend.Admin.Kind == pluginMgr.FrontendKindStatic && p.Paths.FrontendAdminDir != ""
	adminURL := ""
	if hasAdmin {
		adminURL = prefix + "/" + p.ID + "/admin/"
	}

	// 统一输出结构（贴近列表项 + 运行状态）
	out := gin.H{
		"id":          p.ID,
		"name":        p.Name,
		"version":     p.Version,
		"state":       string(p.State),
		"adminURL":    adminURL,
		"apiBase":     prefix + "/" + p.ID + "/api",
		"hasAdmin":    hasAdmin,
		"description": p.Description,
		"author":      p.Metadata.Author,
		"category":    p.Metadata.Category,
		"tags":        append([]string(nil), p.Metadata.Tags...),
		"runtime": gin.H{
			"pid":           proc.PID,
			"port":          proc.Port,
			"state":         proc.State,
			"healthy":       proc.Healthy,
			"restarts":      proc.Restarts,
			"started_at":    proc.StartedAt,
			"stopped_at":    proc.StoppedAt,
			"last_exit_err": proc.LastExitErr,
			"health_ok":     proc.HealthOKCount,
			"health_fails":  proc.HealthFails,
		},
	}

	dtoRequest.ResponseSuccess(c, out)
}

// POST /api/.../admin/plugins/:id/restart
func PluginRestartHandler(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "缺少插件ID", nil)
		return
	}
	mgr := manager.GetPluginManager()

	if err := mgr.Disable(c, id); err != nil {
		dtoRequest.ResponseError(c, statusFromManagerErr(err), "重启插件失败（停用阶段）", err)
		return
	}
	if err := mgr.Enable(c, id); err != nil {
		dtoRequest.ResponseError(c, statusFromManagerErr(err), "重启插件失败（启用阶段）", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
}

// 你可以在 pluginMgr 里提供 Code→HTTP 的映射；这里先最小化映射为 400
func statusFromManagerErr(error) int { return http.StatusBadRequest }
