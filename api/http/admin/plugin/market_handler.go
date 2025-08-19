// api/http/admin/plugin/market_handler.go
package plugin

import (
	mgrimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
	pmdto "github.com/ArtisanCloud/PowerX/pkg/dto/plugin_mgr"
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
		mgr := mgrimpl.GetPluginManager()
		list, err := mgr.List(c)
		if err != nil {
			dto.ResponseError(c, 500, "加载插件失败", err)
			return
		}

		out := make([]pmdto.MarketplaceItem, 0, len(list))
		for _, p := range list {
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

		dto.ResponseSuccess(c, gin.H{"plugins": out})
	}
}
