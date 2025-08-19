package menu

import (
	admdto "github.com/ArtisanCloud/PowerX/api/http/admin/dto"
	"github.com/ArtisanCloud/PowerX/api/http/admin/plugin"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/rbac"
	"github.com/gin-gonic/gin"
	"sort"
)

// GET /api/admin/menus  —— 合并系统+插件菜单，并按权限过滤
func AdminMenusHandler(c *gin.Context) {
	sys := BuildSystemMenus()

	plug := plugin.BuildPluginMenusPublic(plugin.MarketBasePrefix) // 导出一个 wrapper 返回 buildPluginMenus()

	merged := append(sys, plug...)

	// RBAC 过滤：若菜单声明了 permissions，则必须全部满足
	allow := func(perms []string) bool {
		if len(perms) == 0 {
			return true
		}
		for _, pol := range perms {
			// pol 形如 "resource:action"
			res, act := splitPolicy(pol)
			if !rbac.Check(c, res, act) { // 你已有的 rbac.Checker
				return false
			}
		}
		return true
	}

	out := make([]admdto.AdminMenuItem, 0, len(merged))
	for _, m := range merged {
		if allow(m.Permissions) {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Order < out[j].Order })

	dto.ResponseSuccess(c, gin.H{"menus": out})
}

func splitPolicy(p string) (string, string) {
	for i := 0; i < len(p); i++ {
		if p[i] == ':' {
			return p[:i], p[i+1:]
		}
	}
	return p, "*" // 容错
}
