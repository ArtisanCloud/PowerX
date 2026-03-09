// api/http/admin/plugin/market_handler.go
package plugin

import (
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
	pmdto "github.com/ArtisanCloud/PowerX/pkg/dto/plugin_mgr"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
	"os"
	"path/filepath"
	"strings"
)

// ---- 基础配置（由路由层注入一次） ----
var MarketBasePrefix = "/_p"

// SetMarketplaceBasePrefix 由启动/路由注册时调用一次
func SetMarketplaceBasePrefix(p string) {
	if p == "" {
		p = "/_p"
	}
	MarketBasePrefix = strings.TrimRight(p, "/")
}

// 工厂：注入 basePrefix（例如 "/_p"），返回 gin.HandlerFunc
func MarketplaceListHandler(basePrefix string) gin.HandlerFunc {

	getInstallsCount := func(_ string) int64 { return 0 } // TODO: 接入真实统计

	// 优先尝试把 manifest.metadata.icon 映射到 admin 静态目录
	resolveIconURL := func(pID, adminDirAbs, iconFromMeta string) string {
		if iconFromMeta == "" {
			// 回退：尝试常见文件名
			for _, rel := range []string{"icon.svg", "icon.png", "assets/icon.svg", "assets/icon.png"} {
				if fi, err := os.Stat(filepath.Join(adminDirAbs, rel)); err == nil && !fi.IsDir() {
					return basePrefix + "/" + pID + "/admin/" + rel
				}
			}
			return ""
		}
		// 规范化相对路径（去掉 "./" 前缀）
		rel := strings.TrimPrefix(iconFromMeta, "./")
		// 如果作者把 icon 放在插件根的 assets/ 下，而不是 admin 静态目录，
		// 这时你暂时无法通过 admin 静态路由访问。建议将 icon 放到 frontend/admin 下。
		// 这里我们先尝试在 adminDir 里找这个相对路径：
		if fi, err := os.Stat(filepath.Join(adminDirAbs, rel)); err == nil && !fi.IsDir() {
			return basePrefix + "/" + pID + "/admin/" + rel
		}
		// 允许直接给 http(s) 链接
		if strings.HasPrefix(iconFromMeta, "http://") || strings.HasPrefix(iconFromMeta, "https://") {
			return iconFromMeta
		}
		return ""
	}

	return func(c *gin.Context) {
		mgr, err := tryGetPluginManager()
		var list []pmdto.MarketplaceItem
		var plugs []plugin_mgr.Plugin
		if err == nil {
			plugs, err = mgr.List(c)
			if err != nil {
				dto.ResponseError(c, 500, "加载插件失败", err)
				return
			}
		} else {
			plugs, err = listPluginsFromRegistry(c)
			if err != nil {
				dto.ResponseList(c, []pmdto.MarketplaceItem{}, nil)
				return
			}
		}

		out := make([]pmdto.MarketplaceItem, 0, len(plugs))
		for _, p := range plugs {
			iconURL := ""
			if p.Frontend.Admin.Kind == "static" && p.Paths.FrontendAdminDir != "" {
				if abs, err := filepath.Abs(p.Paths.FrontendAdminDir); err == nil {
					iconURL = resolveIconURL(p.ID, abs, p.Metadata.Icon)
				}
			}

			out = append(out, pmdto.MarketplaceItem{
				ID:          p.ID,
				Name:        p.Name,        // ★
				Description: p.Description, // ★
				Version:     p.Version,
				Author:      p.Metadata.Author,   // ★
				Category:    p.Metadata.Category, // ★
				Installs:    getInstallsCount(p.ID),
				Icon:        iconURL,                                   // 尝试映射 admin 静态
				Tags:        append([]string(nil), p.Metadata.Tags...), // 避免 nil → null
			})
		}

		list = out
		dto.ResponseList(c, list, nil)
	}
}

// MarketplaceListV2Handler: 返回贴合前端 Plugin 类型的结构（本地模拟）
// 说明：无远端 Marketplace 时，使用当前已安装列表 + 元数据补齐字段；后续可改为远端合并。
func MarketplaceListV2Handler(basePrefix string) gin.HandlerFunc {
	// 复用与 v1 相同的图标解析逻辑
	resolveIconURL := func(pID, adminDirAbs, iconFromMeta string) string {
		if iconFromMeta == "" {
			for _, rel := range []string{"icon.svg", "icon.png", "assets/icon.svg", "assets/icon.png"} {
				if fi, err := os.Stat(filepath.Join(adminDirAbs, rel)); err == nil && !fi.IsDir() {
					return basePrefix + "/" + pID + "/admin/" + rel
				}
			}
			return ""
		}
		rel := strings.TrimPrefix(iconFromMeta, "./")
		if fi, err := os.Stat(filepath.Join(adminDirAbs, rel)); err == nil && !fi.IsDir() {
			return basePrefix + "/" + pID + "/admin/" + rel
		}
		if strings.HasPrefix(iconFromMeta, "http://") || strings.HasPrefix(iconFromMeta, "https://") {
			return iconFromMeta
		}
		return ""
	}

	slugify := func(id string) string {
		s := strings.ReplaceAll(id, ".", "-")
		s = strings.ReplaceAll(s, "_", "-")
		return s
	}

	// map state → systemStatus
	toSystemStatus := func(state string) string {
		switch strings.ToLower(strings.TrimSpace(state)) {
		case "enabled":
			return "enabled"
		case "disabled":
			return "disabled"
		case "installed":
			return "installed"
		case "broken", "invalid", "abnormal":
			return "broken"
		default:
			return "not_installed"
		}
	}

	return func(c *gin.Context) {
		mgr, err := tryGetPluginManager()
		var list []plugin_mgr.Plugin
		if err == nil {
			list, err = mgr.List(c)
			if err != nil {
				dto.ResponseError(c, 500, "加载插件失败", err)
				return
			}
		} else {
			list, err = listPluginsFromRegistry(c)
			if err != nil {
				dto.ResponseList(c, []gin.H{}, nil)
				return
			}
		}

		out := make([]gin.H, 0, len(list))
		for _, p := range list {
			iconURL := ""
			if p.Frontend.Admin.Kind == "static" && p.Paths.FrontendAdminDir != "" {
				if abs, err := filepath.Abs(p.Paths.FrontendAdminDir); err == nil {
					iconURL = resolveIconURL(p.ID, abs, p.Metadata.Icon)
				}
			}

			systemStatus := toSystemStatus(string(p.State))
			isInstalled := systemStatus != "not_installed"
			isEnabled := systemStatus == "enabled"

			// 补齐前端需要的字段；无远端 marketplace 时给默认值
			item := gin.H{
				"id":                p.ID,
				"name":              p.Name,
				"slug":              slugify(p.ID),
				"version":           p.Version,
				"description":       p.Description,
				"author":            p.Metadata.Author,
				"authorUrl":         p.Metadata.Homepage,
				"homepage":          p.Metadata.Homepage,
				"repository":        "",
				"license":           p.Metadata.License,
				"icon":              iconURL,
				"screenshots":       []string{},
				"category":          p.Metadata.Category,
				"tags":              append([]string(nil), p.Metadata.Tags...),
				"systemStatus":      systemStatus,
				"isSystemInstalled": isInstalled,
				"isSystemEnabled":   isEnabled,
				"installPath":       p.Paths.Root,
				"configSchema":      gin.H{},
				"config":            gin.H{},
				"dependencies":      []string{},
				"requirements":      gin.H{},
				"downloadUrl":       "",
				"downloadCount":     0,
				"rating":            0,
				"reviewCount":       0,
				"lastUpdated":       "",
				"createdAt":         "",
				"updatedAt":         "",
			}
			out = append(out, item)
		}

		// 使用统一列表响应格式
		dto.ResponseList(c, out, nil)
	}
}
