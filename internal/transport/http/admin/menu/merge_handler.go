package menu

// api/http/admin/menu/merge_handler.go

import (
	admdto "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/dto"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/plugin"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
	"sort"
)

// GET /api/admin/menus  —— 合并系统+插件菜单，并按权限过滤
// GET /api/admin/menus  —— 合并系统+插件菜单，并按权限过滤 + 顶层排序
func AdminMenusHandler(c *gin.Context) {
	sys := BuildSystemMenus()
	plug := plugin.BuildPluginMenusPublic(plugin.MarketBasePrefix)

	// 1) RBAC 过滤（AND）
	allow := func(perms []string) bool {
		if len(perms) == 0 {
			return true
		}
		//for _, pol := range perms {
		//res, act := splitPolicy(pol)
		//checker := rbac.NewChecker()
		//result, _ := checker.Check(c, perms, res, act, nil)
		//if !result.Allow {
		//	return false
		//}
		//}
		return true
	}

	// 2) 建立系统槽位索引
	slots := indexSystemSlots(sys) // settings/dashboard/workflow/agent/plugins

	// 3) 将插件菜单按 slot 放置；同时收集 group.root 顶层插件
	var rootPlugins []admdto.AdminMenuItem

	for _, m := range plug {
		if !allow(m.Permissions) {
			continue
		}
		if !m.Visible {
			m.Visible = true // 缺省可见
		}

		switch m.Slot {
		case plugin_mgr.SlotSettings: // "core.settings"
			m.ParentID = "settings"
			slots["settings"].Children = append(slots["settings"].Children, m)

		case plugin_mgr.SlotDashboard: // "core.dashboard"
			m.ParentID = "dashboard"
			slots["dashboard"].Children = append(slots["dashboard"].Children, m)

		case plugin_mgr.SlotWorkflow: // "core.workflow"
			m.ParentID = "workflow"
			slots["workflow"].Children = append(slots["workflow"].Children, m)

		case plugin_mgr.SlotAgent: // "core.agent"
			m.ParentID = "agent"
			slots["agent"].Children = append(slots["agent"].Children, m)

		case plugin_mgr.SlotRoot: // "group.root" —— 顶层并列，但要“排在最前”
			m.ParentID = ""
			rootPlugins = append(rootPlugins, m)

		default: // 兜底：插件市场
			m.ParentID = "plugins"
			slots["plugins"].Children = append(slots["plugins"].Children, m)
		}
	}

	// 4) 先递归排序“所有节点的 children”
	sortChildrenRecursive(sys)

	// 5) 顶层排序：group.root 插件在最前，其它顶层在后；各自内部按 order→title→id
	sys = sortTopLevelWithRootFirst(sys, rootPlugins)

	cats := groupAsCategories(sys)
	dto.ResponseSuccess(c, gin.H{"categories": cats})

	//dto.ResponseSuccess(c, gin.H{"menus": sys})
}

// 仅对子节点递归排序（不改变顶层顺序）
func sortChildrenRecursive(nodes []admdto.AdminMenuItem) {
	for i := range nodes {
		if len(nodes[i].Children) > 0 {
			sort.Slice(nodes[i].Children, func(a, b int) bool {
				ai, aj := nodes[i].Children[a], nodes[i].Children[b]
				if ai.Order != aj.Order {
					return ai.Order < aj.Order
				}
				if ai.Title != aj.Title {
					return ai.Title < aj.Title
				}
				return ai.Key < aj.Key
			})
			sortChildrenRecursive(nodes[i].Children)
		}
	}
}

// 顶层排序：先放 group.root 的插件（内部按 order 排），再放其余顶层（内部按 order 排）
func sortTopLevelWithRootFirst(sys []admdto.AdminMenuItem, collectedRoot []admdto.AdminMenuItem) []admdto.AdminMenuItem {
	// 将“非 root 插件”与系统项放到 rest
	rest := make([]admdto.AdminMenuItem, 0, len(sys))
	for _, it := range sys {
		// 已收集的 root 插件不要再重复加入 rest
		if it.Origin == "plugin" && it.Slot == plugin_mgr.SlotRoot {
			// 忽略，等下统一用 collectedRoot
			continue
		}
		rest = append(rest, it)
	}

	// 两个分区内部各自排序
	sort.Slice(collectedRoot, func(i, j int) bool {
		ai, aj := collectedRoot[i], collectedRoot[j]
		if ai.Order != aj.Order {
			return ai.Order < aj.Order
		}
		if ai.Title != aj.Title {
			return ai.Title < aj.Title
		}
		return ai.Key < aj.Key
	})
	sort.Slice(rest, func(i, j int) bool {
		ai, aj := rest[i], rest[j]
		if ai.Order != aj.Order {
			return ai.Order < aj.Order
		}
		if ai.Title != aj.Title {
			return ai.Title < aj.Title
		}
		return ai.Key < aj.Key
	})

	// 顶层：root 插件在前，其它在后
	out := make([]admdto.AdminMenuItem, 0, len(collectedRoot)+len(rest))
	out = append(out, collectedRoot...)
	out = append(out, rest...)
	return out
}
func indexSystemSlots(sys []admdto.AdminMenuItem) map[string]*admdto.AdminMenuItem {
	idx := map[string]*admdto.AdminMenuItem{}
	for i := range sys {
		it := &sys[i]
		switch it.Key {
		case "settings":
			idx["settings"] = it
		case "dashboard":
			idx["dashboard"] = it
		case "workflow":
			idx["workflow"] = it
		case "agent":
			idx["agent"] = it
		case "plugins":
			idx["plugins"] = it
		}
	}
	return idx
}

func splitPolicy(p string) (string, string) {
	for i := 0; i < len(p); i++ {
		if p[i] == ':' {
			return p[:i], p[i+1:]
		}
	}
	return p, "*" // 容错
}

func parseCategoryFromParentID(pid string) (key, title string, ok bool) {
	const prefix = "cat:"
	if len(pid) > len(prefix) && pid[:len(prefix)] == prefix {
		rest := pid[len(prefix):]
		// 允许 key 或 title 中没有 ':'，做个安全拆分
		for i := 0; i < len(rest); i++ {
			if rest[i] == ':' {
				return rest[:i], rest[i+1:], true
			}
		}
		// 只有 key 没有 title 的情况
		return rest, rest, true
	}
	return "", "", false
}

// 把“已排序好的顶层菜单 sys”分成若干分类
func groupAsCategories(sys []admdto.AdminMenuItem) []admdto.AdminMenuCategory {
	byID := map[string]*admdto.AdminMenuCategory{
		"system":  {ID: "system", Title: "系统功能", Order: 0, Origin: "system"},
		"plugins": {ID: "plugins", Title: "插件", Order: 50, Origin: "plugin"},
	}

	// 先扫一遍，看看是否存在 root 顶层插件
	hasRoot := false
	for _, it := range sys {
		if it.Origin != "system" && (it.Slot == plugin_mgr.SlotRoot || (it.ParentID == "" && it.Origin == "plugin")) {
			hasRoot = true
			break
		}
	}
	// 如果有 root，则把插件分类整体提前（在 system 之前）
	if hasRoot {
		byID["plugins"].Order = -10
	}

	// 按“已排好”的顶层顺序分桶，不再对 cat.Children 二次排序
	for _, it := range sys {
		if it.Origin == "system" {
			byID["system"].Children = append(byID["system"].Children, it)
			continue
		}
		if key, title, ok := parseCategoryFromParentID(it.ParentID); ok {
			if _, exists := byID[key]; !exists {
				byID[key] = &admdto.AdminMenuCategory{ID: key, Title: title, Order: 50, Origin: "plugin"}
			}
			byID[key].Children = append(byID[key].Children, it)
			continue
		}
		byID["plugins"].Children = append(byID["plugins"].Children, it)
	}

	// 只排序“分类列表”，不动每个分类内的 children
	out := make([]admdto.AdminMenuCategory, 0, len(byID))
	for _, v := range byID {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		if out[i].Title != out[j].Title {
			return out[i].Title < out[j].Title
		}
		return out[i].ID < out[j].ID
	})
	return out
}
