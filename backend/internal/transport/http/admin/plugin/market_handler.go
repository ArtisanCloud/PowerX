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

func resolveMarketplaceIconURL(basePrefix string, p plugin_mgr.Plugin) string {
	basePrefix = strings.TrimRight(strings.TrimSpace(basePrefix), "/")
	if basePrefix == "" {
		basePrefix = "/_p"
	}

	iconFromMeta := strings.TrimSpace(p.Metadata.Icon)
	if strings.HasPrefix(iconFromMeta, "http://") || strings.HasPrefix(iconFromMeta, "https://") {
		return iconFromMeta
	}

	candidates := make([]string, 0, 3)
	if dir := strings.TrimSpace(p.Paths.FrontendAdminDir); dir != "" {
		if abs, err := filepath.Abs(dir); err == nil {
			candidates = append(candidates, abs)
		}
	}
	if root := strings.TrimSpace(p.Paths.Root); root != "" {
		if staticDir := strings.TrimSpace(p.Frontend.Admin.StaticDir); staticDir != "" {
			if abs, err := filepath.Abs(filepath.Join(root, staticDir)); err == nil {
				candidates = append(candidates, abs)
			}
		}
		if abs, err := filepath.Abs(filepath.Join(root, "web-admin/.output/public")); err == nil {
			candidates = append(candidates, abs)
		}
	}

	rels := []string{}
	if iconFromMeta != "" {
		rels = append(rels, strings.TrimPrefix(iconFromMeta, "./"))
	} else {
		rels = append(rels, "icon.svg", "icon.png", "assets/icon.svg", "assets/icon.png")
	}

	for _, dir := range candidates {
		if dir == "" {
			continue
		}
		for _, rel := range rels {
			if rel == "" || filepath.IsAbs(rel) {
				continue
			}
			if fi, err := os.Stat(filepath.Join(dir, rel)); err == nil && !fi.IsDir() {
				return basePrefix + "/" + p.ID + "/admin/" + filepath.ToSlash(rel)
			}
		}
	}
	return ""
}

// 工厂：注入 basePrefix（例如 "/_p"），返回 gin.HandlerFunc
func MarketplaceListHandler(basePrefix string) gin.HandlerFunc {

	getInstallsCount := func(_ string) int64 { return 0 } // TODO: 接入真实统计

	return func(c *gin.Context) {
		ctx := c.Request.Context()
		mgr, err := tryGetPluginManager()
		var list []pmdto.MarketplaceItem
		var plugs []plugin_mgr.Plugin
		if err == nil {
			plugs, err = mgr.List(ctx)
			if err != nil {
				dto.ResponseError(c, 500, "加载插件失败", err)
				return
			}
		} else {
			plugs, err = listPluginsFromRegistry(ctx)
			if err != nil {
				dto.ResponseList(c, []pmdto.MarketplaceItem{}, nil)
				return
			}
		}

		out := make([]pmdto.MarketplaceItem, 0, len(plugs))
		for _, p := range plugs {
			iconURL := resolveMarketplaceIconURL(basePrefix, p)

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
		ctx := c.Request.Context()
		mgr, err := tryGetPluginManager()
		var list []plugin_mgr.Plugin
		if err == nil {
			list, err = mgr.List(ctx)
			if err != nil {
				dto.ResponseError(c, 500, "加载插件失败", err)
				return
			}
		} else {
			list, err = listPluginsFromRegistry(ctx)
			if err != nil {
				dto.ResponseList(c, []gin.H{}, nil)
				return
			}
		}

		out := make([]gin.H, 0, len(list))
		for _, p := range list {
			iconURL := resolveMarketplaceIconURL(basePrefix, p)

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
