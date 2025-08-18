package admin

import (
	admindto "github.com/ArtisanCloud/PowerX/api/http/admin/dto"
	pluginDto "github.com/ArtisanCloud/PowerX/pkg/dto/plugin_mgr"
	"net/http"
	"sort"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
)

type PluginHandler struct {
	Mgr        plugin_mgr.Manager
	BasePrefix string // cfg.Plugin.BasePrefix，通常 "/_p"
}

func NewPluginHandler(mgr plugin_mgr.Manager, basePrefix string) *PluginHandler {
	return &PluginHandler{Mgr: mgr, BasePrefix: basePrefix}
}

// GET /api/v1/admin/plugins
func (h *PluginHandler) List(c *gin.Context) {
	list, err := h.Mgr.List(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	out := make([]pluginDto.PluginItem, 0, len(list))
	for _, p := range list {
		adminURL := ""
		hasAdmin := p.Frontend.Admin.Kind == plugin_mgr.FrontendKindStatic && p.Paths.FrontendAdminDir != ""
		if hasAdmin {
			adminURL = h.BasePrefix + "/" + p.ID + "/admin/"
		}
		// 拼菜单（把 manifest 里的菜单映射成可点击 URL）
		menus := make([]admindto.PluginMenuItem, 0, len(p.Frontend.Admin.Menus))
		for _, m := range p.Frontend.Admin.Menus {
			menus = append(menus, admindto.PluginMenuItem{
				ID:    p.ID,
				Title: m.Title,
				Icon:  m.Icon,
				Order: m.Order,
				// 简单做法：所有菜单都指向插件根页面（内部再用前端路由）
				URL: adminURL,
			})
		}
		sort.Slice(menus, func(i, j int) bool { return menus[i].Order < menus[j].Order })

		out = append(out, pluginDto.PluginItem{
			ID:       p.ID,
			Name:     p.ID, // 如果你没有该方法，就用 p.ID 或 p.Manifest.Name
			Version:  p.Version,
			State:    string(p.State),
			AdminURL: adminURL,
			APIBase:  h.BasePrefix + "/" + p.ID + "/api",
			HasAdmin: hasAdmin,
			Menus:    menus,
		})
	}
	// 固定排序：启用优先 + 名称
	sort.Slice(out, func(i, j int) bool {
		if out[i].State != out[j].State {
			return out[i].State == string(plugin_mgr.StateEnabled)
		}
		return out[i].ID < out[j].ID
	})
	c.JSON(http.StatusOK, gin.H{"plugins": out})
}

// POST /api/v1/admin/plugins/:id/enable
func (h *PluginHandler) Enable(c *gin.Context) {
	id := c.Param("id")
	if err := h.Mgr.Enable(c, id); err != nil {
		c.JSON(statusFromManagerErr(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// POST /api/v1/admin/plugins/:id/disable
func (h *PluginHandler) Disable(c *gin.Context) {
	id := c.Param("id")
	if err := h.Mgr.Disable(c, id); err != nil {
		c.JSON(statusFromManagerErr(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GET /api/v1/admin/plugins/menus  —— 聚合所有插件菜单（给前端 Admin 左侧栏）
func (h *PluginHandler) Menus(c *gin.Context) {
	list, err := h.Mgr.List(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	agg := make([]admindto.PluginMenuItem, 0, 16)
	for _, p := range list {
		if p.State != plugin_mgr.StateEnabled {
			continue
		}
		if !(p.Frontend.Admin.Kind == plugin_mgr.FrontendKindStatic && p.Paths.FrontendAdminDir != "") {
			continue
		}
		base := h.BasePrefix + "/" + p.ID + "/admin/"
		for _, m := range p.Frontend.Admin.Menus {
			agg = append(agg, admindto.PluginMenuItem{
				ID:    p.ID,
				Title: m.Title,
				Icon:  m.Icon,
				Order: m.Order,
				URL:   base, // 简化：全部打开插件根；如需更细路由，可用 hash 参数
			})
		}
	}
	sort.Slice(agg, func(i, j int) bool { return agg[i].Order < agg[j].Order })
	c.JSON(http.StatusOK, gin.H{"menus": agg})
}

func statusFromManagerErr(err error) int {
	// 如果你在 plugin_mgr 里有 Code→HTTPStatus 的映射函数可直接用
	// 这里最小实现：启停相关错误默认 400/500
	return http.StatusBadRequest
}
