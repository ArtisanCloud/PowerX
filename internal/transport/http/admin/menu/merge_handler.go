package menu

// api/http/admin/menu/merge_handler.go

import (
	admdto "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/dto"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/plugin"
	"github.com/ArtisanCloud/PowerX/pkg/corex/rbac"
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
		sub := rbac.SubjectFromContext(c)

		for _, pol := range perms {
			res, act := splitPolicy(pol)
			checker := rbac.NewChecker()
			result, _ := checker.Check(c, sub, res, act, nil)
			if !result.Allow {
				return false
			}
		}
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
			m.ParentID = plugin_mgr.KeySettings
			slots[plugin_mgr.KeySettings].Children = append(slots[plugin_mgr.KeySettings].Children, m)

		case plugin_mgr.SlotDashboard: // "core.dashboard"
			m.ParentID = plugin_mgr.KeyDashboard
			slots[plugin_mgr.KeyDashboard].Children = append(slots[plugin_mgr.KeyDashboard].Children, m)

		case plugin_mgr.SlotWorkflow: // "core.workflow"
			m.ParentID = plugin_mgr.KeyWorkflow
			slots[plugin_mgr.KeyWorkflow].Children = append(slots[plugin_mgr.KeyWorkflow].Children, m)

		case plugin_mgr.SlotAgent: // "core.agent"
			m.ParentID = plugin_mgr.KeyAgent
			slots[plugin_mgr.KeyAgent].Children = append(slots[plugin_mgr.KeyAgent].Children, m)

		case plugin_mgr.SlotRoot: // "group.root" —— 顶层并列，但要“排在最前”
			m.ParentID = ""
			rootPlugins = append(rootPlugins, m)

		default: // 兜底：插件市场
			m.ParentID = plugin_mgr.KeyPlugins
			slots[plugin_mgr.KeyPlugins].Children = append(slots[plugin_mgr.KeyPlugins].Children, m)
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
		if it.Origin == plugin_mgr.OriginPlugin && it.Slot == plugin_mgr.SlotRoot {
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
func indexSystemSlots(sys []admdto.AdminMenuItem) map[plugin_mgr.MenuKey]*admdto.AdminMenuItem {
	idx := map[plugin_mgr.MenuKey]*admdto.AdminMenuItem{}
	for i := range sys {
		it := &sys[i]
		switch it.Key {
		case plugin_mgr.KeySettings:
			idx[plugin_mgr.KeySettings] = it
		case plugin_mgr.KeyDashboard:
			idx[plugin_mgr.KeyDashboard] = it
		case plugin_mgr.KeyWorkflow:
			idx[plugin_mgr.KeyWorkflow] = it
		case plugin_mgr.KeyAgent:
			idx[plugin_mgr.KeyAgent] = it
		case plugin_mgr.KeyPlugins:
			idx[plugin_mgr.KeyPlugins] = it
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
	byID := map[plugin_mgr.MenuKey]*admdto.AdminMenuCategory{
		plugin_mgr.KeyAgent:    {ID: plugin_mgr.KeyAgent, Title: "智能体", Order: -100, Origin: plugin_mgr.OriginSystem},
		plugin_mgr.KeyWorkflow: {ID: plugin_mgr.KeyWorkflow, Title: "工作流", Order: -90, Origin: plugin_mgr.OriginSystem},
		plugin_mgr.KeyPlugins:  {ID: plugin_mgr.KeyPlugins, Title: "插件", Order: -10, Origin: plugin_mgr.OriginSystem},
		plugin_mgr.KeySettings: {ID: plugin_mgr.KeySettings, Title: "系统设置", Order: 0, Origin: plugin_mgr.OriginSystem},
	}

	for _, it := range sys {
		// system 顶层：先分发 agent / workflow，再落 system
		if it.Origin == plugin_mgr.OriginSystem {
			switch it.Key {
			case plugin_mgr.KeyAgent:
				byID[plugin_mgr.KeyAgent].Children = append(byID[plugin_mgr.KeyAgent].Children, it)
				continue
			case plugin_mgr.KeyWorkflow:
				byID[plugin_mgr.KeyWorkflow].Children = append(byID[plugin_mgr.KeyWorkflow].Children, it)
				continue
			case plugin_mgr.KeyPlugins:
				// 关键：把系统“插件市场”放到“插件”分类下，而不是系统设置
				byID[plugin_mgr.KeyPlugins].Children = append(byID[plugin_mgr.KeyPlugins].Children, it)
				continue
			default:
				byID[plugin_mgr.KeySettings].Children = append(byID[plugin_mgr.KeySettings].Children, it)
				continue
			}
		}

		// 插件：保持 cat: 自定义分类逻辑；否则落到 plugins
		if strKey, title, ok := parseCategoryFromParentID(string(it.ParentID)); ok {
			key := plugin_mgr.MenuKey(strKey)
			if _, exists := byID[key]; !exists {
				byID[key] = &admdto.AdminMenuCategory{ID: key, Title: title, Order: 50, Origin: plugin_mgr.OriginPlugin}
			}
			byID[key].Children = append(byID[key].Children, it)
			continue
		}
		byID[plugin_mgr.KeyPlugins].Children = append(byID[plugin_mgr.KeyPlugins].Children, it)
	}

	// 分类列表排序：仅按 Order→Title→ID；不再基于 hasRoot 调整 plugins 的顺序
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
