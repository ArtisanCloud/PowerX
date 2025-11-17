package menu

// api/http/admin/menu/merge_handler.go

import (
	"log"
	"sort"
	"strings"

	admdto "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/dto"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/plugin"
	"github.com/ArtisanCloud/PowerX/pkg/corex/rbac"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
)

// 日志开关：调试时设为 true；上线请改为 false。
const i18nDebug = false

func dbgI18n(format string, args ...any) {
	if i18nDebug {
		log.Printf("[i18n] "+format, args...)
	}
}

type pluginGroup struct {
	Key   plugin_mgr.MenuKey
	Title string
	Order int
	Items []admdto.AdminMenuItem
}

// ------- 工具：去重追加 -------
func uniqAppend(list []string, s string) []string {
	if s == "" {
		return list
	}
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}

// ---------- locale 优先级与标准化 ----------

// 构建偏好列表：["zh-CN","zh",...,"en-US","en",...]
func buildPreferredLocales(locales []string) []string {
	preferred := make([]string, 0, len(locales)*3)
	seen := map[string]struct{}{}

	add := func(loc string) {
		loc = strings.TrimSpace(loc)
		if loc == "" {
			return
		}
		if _, ok := seen[loc]; ok {
			return
		}
		seen[loc] = struct{}{}
		preferred = append(preferred, loc)
	}

	for _, loc := range locales {
		add(loc)
		// zh-CN -> zh
		if parts := strings.Split(loc, "-"); len(parts) > 1 {
			add(parts[0])
		}
		// 兼容大小写与 -/_ 变体
		add(strings.ToLower(strings.ReplaceAll(loc, "_", "-")))
		add(strings.ToLower(strings.ReplaceAll(loc, "-", "_")))
	}
	return preferred
}

func normalizeLocaleTag(s string) string {
	if s == "" {
		return ""
	}
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "_", "-")
	return s
}

// 从 bundle.Locales 中，根据期望 locale 找最合适的 map（容错大小写、-/_、同语种回退）
func resolveLocaleMap(locales admdto.MenuI18nLocales, wanted string) (map[string]map[string]any, bool) {
	if len(locales) == 0 {
		return nil, false
	}
	// 1) 直接命中
	if m, ok := locales[wanted]; ok {
		return m, true
	}
	// 2) 同形异大小写/连字符
	nWanted := normalizeLocaleTag(wanted)
	for k, v := range locales {
		if normalizeLocaleTag(k) == nWanted {
			return v, true
		}
	}
	// 3) 按语种回退（zh-CN -> zh；en-US -> en）
	base := wanted
	if i := strings.IndexAny(wanted, "-_"); i > 0 {
		base = wanted[:i]
	}
	nBase := normalizeLocaleTag(base)

	if v, ok := locales[base]; ok {
		return v, true
	}
	for k, v := range locales {
		if normalizeLocaleTag(k) == nBase || strings.HasPrefix(normalizeLocaleTag(k), nBase) {
			return v, true
		}
	}
	return nil, false
}

// ---------- i18n：分层 JSON 的点路径读取 + 命名空间兼容 ----------

// 在 map[string]any 中按 "a.b.c" 取值
func getByDotted(m map[string]any, dotted string) (string, bool) {
	if m == nil || dotted == "" {
		return "", false
	}
	parts := strings.Split(dotted, ".")
	var cur any = m
	for _, p := range parts {
		mp, ok := cur.(map[string]any)
		if !ok {
			return "", false
		}
		cur, ok = mp[p]
		if !ok {
			return "", false
		}
	}
	if s, ok := cur.(string); ok && s != "" {
		return s, true
	}
	return "", false
}

// menus <-> menu
func altNamespace(ns string) (string, bool) {
	ns = strings.TrimSpace(ns)
	if ns == "" {
		return "", false
	}
	if strings.HasSuffix(ns, "s") && len(ns) > 1 {
		return ns[:len(ns)-1], true
	}
	return ns + "s", true
}

// ---------- 按 key 查：支持 "<ns>.<path>" 与纯路径 ----------
func findTranslationByKey(key string, i18n []admdto.MenuI18nPackage, preferredLocales []string) (string, bool) {
	if key == "" || len(i18n) == 0 || len(preferredLocales) == 0 {
		return "", false
	}

	for _, loc := range preferredLocales {
		for pi, bundle := range i18n {
			namespaces, ok := resolveLocaleMap(bundle.Locales, loc)
			if !ok {
				continue
			}
			// 拆出 ns 前缀（仅用于“按 ns 优先查”）
			var ns, path string
			if idx := strings.IndexByte(key, '.'); idx > 0 {
				ns = key[:idx]
				path = key[idx+1:]
			}

			// 1) 优先：若带 ns，在该 ns 下尝试多候选
			if ns != "" {
				tryNSList := []string{ns}
				if ns2, ok := altNamespace(ns); ok {
					tryNSList = uniqAppend(tryNSList, ns2)
				}
				for _, tryNSName := range tryNSList {
					if nsData, okNS := namespaces[tryNSName]; okNS {
						candidates := []string{}
						candidates = uniqAppend(candidates, key)
						if path != "" {
							candidates = uniqAppend(candidates, path)
							candidates = uniqAppend(candidates, "menu."+path)
							candidates = uniqAppend(candidates, "menus."+path)
						}
						candidates = uniqAppend(candidates, "menu."+key)
						candidates = uniqAppend(candidates, "menus."+key)

						for _, cand := range candidates {
							if s, ok := getByDotted(nsData, cand); ok && s != "" {
								dbgI18n("hit byKey: pkg=%d loc=%s ns=%s cand=%s -> %s", pi, loc, tryNSName, cand, s)
								return s, true
							} else {
								dbgI18n("miss byKey: pkg=%d loc=%s ns=%s cand=%s", pi, loc, tryNSName, cand)
							}
						}
					}
				}
			}

			// 2) 兜底：遍历所有 ns，尝试多候选
			for nsName, nsData := range namespaces {
				candidates := []string{key}
				if strings.HasPrefix(key, "menu.") {
					candidates = uniqAppend(candidates, strings.TrimPrefix(key, "menu."))
				}
				if strings.HasPrefix(key, "menus.") {
					candidates = uniqAppend(candidates, strings.TrimPrefix(key, "menus."))
				}
				candidates = uniqAppend(candidates, "menu."+key)
				candidates = uniqAppend(candidates, "menus."+key)

				for _, cand := range candidates {
					if s, ok := getByDotted(nsData, cand); ok && s != "" {
						dbgI18n("hit byKey(anyNS): pkg=%d loc=%s ns=%s cand=%s -> %s", pi, loc, nsName, cand, s)
						return s, true
					} else {
						dbgI18n("miss byKey(anyNS): pkg=%d loc=%s ns=%s cand=%s", pi, loc, nsName, cand)
					}
				}
			}
		}
	}
	return "", false
}

// ---------- 带 namespace 的查找：尝试多候选 ----------
func findTranslation(label *admdto.MenuI18nLabel, i18n []admdto.MenuI18nPackage, preferredLocales []string) (string, bool) {
	if label == nil || label.Key == "" || len(i18n) == 0 || len(preferredLocales) == 0 {
		return "", false
	}

	tryNS := func(ns, key string) (string, bool) {
		for _, loc := range preferredLocales {
			for pi, bundle := range i18n {
				if namespaces, ok := resolveLocaleMap(bundle.Locales, loc); ok {
					if nsData, ok := namespaces[ns]; ok {
						candidates := []string{key}
						prefix := ns + "."
						if strings.HasPrefix(key, prefix) {
							candidates = uniqAppend(candidates, strings.TrimPrefix(key, prefix))
						}
						if strings.HasPrefix(key, "menu.") {
							candidates = uniqAppend(candidates, strings.TrimPrefix(key, "menu."))
						}
						if strings.HasPrefix(key, "menus.") {
							candidates = uniqAppend(candidates, strings.TrimPrefix(key, "menus."))
						}
						candidates = uniqAppend(candidates, "menu."+key)
						candidates = uniqAppend(candidates, "menus."+key)

						for _, cand := range candidates {
							if s, ok := getByDotted(nsData, cand); ok && s != "" {
								dbgI18n("hit withNS: pkg=%d loc=%s ns=%s cand=%s -> %s", pi, loc, ns, cand, s)
								return s, true
							} else {
								dbgI18n("miss withNS: pkg=%d loc=%s ns=%s cand=%s", pi, loc, ns, cand)
							}
						}
					}
				}
			}
		}
		return "", false
	}

	ns := strings.TrimSpace(label.Namespace)
	key := strings.TrimSpace(label.Key)

	if ns != "" {
		if s, ok := tryNS(ns, key); ok {
			return s, true
		}
		if ns2, ok := altNamespace(ns); ok && ns2 != ns {
			if s, ok := tryNS(ns2, key); ok {
				return s, true
			}
		}
	}

	// 兜底：忽略 ns，所有命名空间都尝试
	for _, loc := range preferredLocales {
		for pi, bundle := range i18n {
			if namespaces, ok := resolveLocaleMap(bundle.Locales, loc); ok {
				for nsName, nsData := range namespaces {
					candidates := []string{key}
					if strings.HasPrefix(key, "menu.") {
						candidates = uniqAppend(candidates, strings.TrimPrefix(key, "menu."))
					}
					if strings.HasPrefix(key, "menus.") {
						candidates = uniqAppend(candidates, strings.TrimPrefix(key, "menus."))
					}
					candidates = uniqAppend(candidates, "menu."+key)
					candidates = uniqAppend(candidates, "menus."+key)

					for _, cand := range candidates {
						if s, ok := getByDotted(nsData, cand); ok && s != "" {
							dbgI18n("hit anyNS: pkg=%d loc=%s ns=%s cand=%s -> %s", pi, loc, nsName, cand, s)
							return s, true
						} else {
							dbgI18n("miss anyNS: pkg=%d loc=%s ns=%s cand=%s", pi, loc, nsName, cand)
						}
					}
				}
			}
		}
	}
	return "", false
}

func looksLikeI18nKey(s string) bool {
	if s == "" {
		return false
	}
	if strings.Contains(s, " ") {
		return false
	}
	return strings.Contains(s, ".")
}

// ---------- 从菜单 Key 派生翻译 key / 组头标题 ----------
func extractRouteFromPluginKey(k plugin_mgr.MenuKey) (string, bool) {
	val := string(k)
	if !strings.HasPrefix(val, "plugin:") {
		return "", false
	}
	idx := strings.LastIndexByte(val, ':')
	if idx <= 0 || idx == len(val)-1 {
		return "", false
	}
	return val[idx+1:], true // 如 "base.reports.daily"
}

func deriveI18nKeyFromItemKey(k plugin_mgr.MenuKey) (string, bool) {
	route, ok := extractRouteFromPluginKey(k)
	if !ok || route == "" {
		return "", false
	}
	return "menu." + route, true // 命中 menus.json 的 "menu": {...}
}

func guessPluginGroupTitleFromChildren(children []admdto.AdminMenuItem, i18n []admdto.MenuI18nPackage, locales []string) string {
	if len(children) == 0 {
		return ""
	}
	for _, ch := range children {
		route, ok := extractRouteFromPluginKey(ch.Key)
		if !ok || route == "" {
			continue
		}
		if idx := strings.IndexByte(route, '.'); idx > 0 {
			prefix := route[:idx] // e.g. "base"
			key := "menu." + prefix + ".title"
			if t, ok2 := findTranslationByKey(key, i18n, buildPreferredLocales(locales)); ok2 && t != "" {
				return t
			}
		}
	}
	return ""
}

// ---------- 递归翻译 ----------
func translateMenuItemsRecursive(nodes []admdto.AdminMenuItem, i18n []admdto.MenuI18nPackage, locales []string) {
	preferredLocales := buildPreferredLocales(locales)
	hasI18n := len(i18n) > 0 && len(preferredLocales) > 0

	var translate func(nodes []admdto.AdminMenuItem)
	translate = func(nodes []admdto.AdminMenuItem) {
		for i := range nodes {
			node := &nodes[i]
			translated := ""
			applied := false

			if node.TitleI18n != nil {
				// 仅当存在 i18n 包时才尝试翻译与兜底
				if hasI18n {
					if t, ok1 := findTranslation(node.TitleI18n, i18n, preferredLocales); ok1 && t != "" {
						translated, applied = t, true
					} else {
						// 失败后，仅当原 Title 看起来是 key 或为空时，才用 Default/Key 兜底
						if looksLikeI18nKey(node.Title) || node.Title == "" {
							if node.TitleI18n.Default != "" {
								translated, applied = node.TitleI18n.Default, true
							} else if node.TitleI18n.Key != "" {
								translated, applied = node.TitleI18n.Key, true
							}
						}
					}
				}
				// 无 i18n 包：绝不覆盖原始 Title（避免把中文覆盖成英文 Default）
			} else {
				// 无 TitleI18n：仅当 Title 像 key，且有 i18n 包时尝试翻译
				if hasI18n && looksLikeI18nKey(node.Title) {
					if t, ok2 := findTranslationByKey(node.Title, i18n, preferredLocales); ok2 && t != "" {
						translated, applied = t, true
					}
				}
				// 兜底利用 ID 派生 key 只在有 i18n 包时尝试
				if !applied && hasI18n {
					if dk, okDerive := deriveI18nKeyFromItemKey(node.Key); okDerive {
						if t, ok3 := findTranslationByKey(dk, i18n, preferredLocales); ok3 && t != "" {
							translated, applied = t, true
						}
					}
				}
			}

			if i18nDebug && strings.HasPrefix(string(node.Key), "plugin:") {
				log.Printf("[i18n] item=%s hasI18n=%v before=%q after=%q applied=%v", node.Key, hasI18n, node.Title, translated, applied)
			}

			if applied && translated != "" {
				node.Title = translated
			}

			if len(node.Children) > 0 {
				translate(node.Children)
			}
		}
	}
	translate(nodes)
}

// ---------- Handler ----------
func AdminMenusHandler(c *gin.Context) {
	sys := BuildSystemMenus()
	locales := parseLocaleQuery(c)
	plug := plugin.BuildPluginMenusPublic(c.Request.Context(), plugin.MarketBasePrefix, locales)

	//log.Printf("[menus] plugin items=%d, i18n=%d", len(plug.Items), len(plug.I18n))

	if i18nDebug {
		log.Printf("[i18n] locales query = %v", locales)
		log.Printf("[i18n] plugin i18n packages = %d", len(plug.I18n))
		for pi, pkg := range plug.I18n {
			var keys []string
			for loc := range pkg.Locales {
				keys = append(keys, loc)
			}
			log.Printf("[i18n] pkg#%d locales: %v", pi, keys)
			for loc, nsMap := range pkg.Locales {
				var nsKeys []string
				for ns := range nsMap {
					nsKeys = append(nsKeys, ns)
				}
				log.Printf("[i18n]   - %s namespaces: %v", loc, nsKeys)
			}
		}
	}

	// 只合并插件 i18n；先解决 cat:market
	allI18n := plug.I18n

	// 1) 权限过滤
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

	_ = indexSystemSlots(sys)

	// 2) 收集插件顶层
	var (
		rootPlugins    []admdto.AdminMenuItem
		pluginTopLevel = make([]admdto.AdminMenuItem, 0, len(plug.Items))
	)
	for _, m := range plug.Items {
		if !allow(m.Permissions) {
			log.Printf("[menus] filtered by RBAC item=%s perms=%v", m.Key, m.Permissions)
			continue
		}
		if !m.Visible {
			m.Visible = true
		}
		switch m.Slot {
		case plugin_mgr.SlotSettings:
		case plugin_mgr.SlotDashboard, plugin_mgr.SlotWorkflow, plugin_mgr.SlotAgent:
		case plugin_mgr.SlotRoot:
			rootPlugins = append(rootPlugins, m)
		}
		pluginTopLevel = append(pluginTopLevel, m)
	}

	// 3) 排序子节点
	sortChildrenRecursive(sys)

	// 4) 合并系统 + 插件
	sys = append(sys, pluginTopLevel...)

	// 5) 翻译（这一步会把 cat:market 的子项在有 i18n 时翻译；无 i18n 时不覆盖）
	translateMenuItemsRecursive(sys, allI18n, locales)

	// 6) 顶层排序
	sys = sortTopLevelWithRootFirst(sys, rootPlugins)

	// 7) 分组
	cats := groupAsCategories(sys, allI18n, locales)
	payload := gin.H{"categories": cats}
	if len(plug.I18n) > 0 {
		payload["i18n"] = plug.I18n
	}
	dto.ResponseSuccess(c, payload)
}

func parseLocaleQuery(c *gin.Context) []string {
	values := c.QueryArray("locale")
	if combined := c.Query("locales"); combined != "" {
		values = append(values, strings.Split(combined, ",")...)
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
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

// 顶层排序：先放 group.root 的插件，再放其余
func sortTopLevelWithRootFirst(sys []admdto.AdminMenuItem, collectedRoot []admdto.AdminMenuItem) []admdto.AdminMenuItem {
	rest := make([]admdto.AdminMenuItem, 0, len(sys))
	for _, it := range sys {
		if it.Origin == plugin_mgr.OriginPlugin && it.Slot == plugin_mgr.SlotRoot {
			continue
		}
		rest = append(rest, it)
	}

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
		case plugin_mgr.KeyKnowledgeSpace:
			idx[plugin_mgr.KeyKnowledgeSpace] = it
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
	return p, "*"
}

func i18nOrDefault(key, def string, i18n []admdto.MenuI18nPackage, locales []string) string {
	preferred := buildPreferredLocales(locales)
	if t, ok := findTranslationByKey(key, i18n, preferred); ok {
		return t
	}
	return def
}

// 分组（含组头中文的猜测逻辑）
func groupAsCategories(sys []admdto.AdminMenuItem, i18n []admdto.MenuI18nPackage, locales []string) []admdto.AdminMenuCategory {
	const (
		catPinnedKey     plugin_mgr.MenuKey = "cat:pinned"
		catAppsKey       plugin_mgr.MenuKey = "cat:market"
		catAppsPrefixKey string             = "cat:market:"
	)

	byID := map[plugin_mgr.MenuKey]*admdto.AdminMenuCategory{
		catPinnedKey:           {ID: catPinnedKey, Title: i18nOrDefault("menu.section.pinned", "Pinned", i18n, locales), Order: -200, Origin: plugin_mgr.OriginSystem},
		catAppsKey:             {ID: catAppsKey, Title: i18nOrDefault("menu.section.apps", "Apps", i18n, locales), Order: -50, Origin: plugin_mgr.OriginPlugin},
		plugin_mgr.KeySettings: {ID: plugin_mgr.KeySettings, Title: i18nOrDefault("menu.section.settings", "Settings", i18n, locales), Order: 0, Origin: plugin_mgr.OriginSystem},
	}

	plugGroups := map[string]*pluginGroup{}

	for _, origin := range sys {
		item := origin
		if item.Origin == plugin_mgr.OriginSystem {
			switch item.Key {
			case plugin_mgr.KeyAgent, plugin_mgr.KeyKnowledgeSpace, plugin_mgr.KeyWorkflow, plugin_mgr.KeyDashboard:
				byID[catPinnedKey].Children = append(byID[catPinnedKey].Children, item)
			case plugin_mgr.KeyPlugins:
				byID[plugin_mgr.KeySettings].Children = append(byID[plugin_mgr.KeySettings].Children, item)
			default:
				byID[plugin_mgr.KeySettings].Children = append(byID[plugin_mgr.KeySettings].Children, item)
			}
			continue
		}

		pluginID := extractPluginIDFromKey(item.Key)
		switch {
		case item.Slot == plugin_mgr.SlotRoot:
			byID[catPinnedKey].Children = append(byID[catPinnedKey].Children, item)
		case item.Slot == plugin_mgr.SlotSettings:
			byID[plugin_mgr.KeySettings].Children = append(byID[plugin_mgr.KeySettings].Children, item)
		case item.Slot == plugin_mgr.SlotDashboard:
			byID[catPinnedKey].Children = append(byID[catPinnedKey].Children, item)
		case strings.HasPrefix(string(item.Slot), string(plugin_mgr.SlotPlugins)+""):
			if pluginID == "" {
				byID[catAppsKey].Children = append(byID[catAppsKey].Children, item)
				continue
			}
			suffix := strings.TrimPrefix(string(item.Slot), string(plugin_mgr.SlotPlugins)+".")
			groupKey := catAppsPrefixKey + pluginID + ":" + suffix
			title := formatSlotSeparatorTitle(suffix, i18n, locales)
			addToPluginGroup(plugGroups, groupKey, title, item)
		default:
			if pluginID == "" {
				byID[catAppsKey].Children = append(byID[catAppsKey].Children, item)
				continue
			}
			groupKey := catAppsPrefixKey + pluginID
			title := formatPluginGroupTitle(item.Title, i18n, locales)
			addToPluginGroup(plugGroups, groupKey, title, item)
		}
	}

	if len(plugGroups) > 0 {
		orderedGroups := make([]*pluginGroup, 0, len(plugGroups))
		for _, g := range plugGroups {
			sort.Slice(g.Items, func(i, j int) bool {
				a, b := g.Items[i], g.Items[j]
				if a.Order != b.Order {
					return a.Order < b.Order
				}
				if a.Title != b.Title {
					return a.Title < b.Title
				}
				return a.Key < b.Key
			})
			orderedGroups = append(orderedGroups, g)
		}
		sort.Slice(orderedGroups, func(i, j int) bool {
			if orderedGroups[i].Order != orderedGroups[j].Order {
				return orderedGroups[i].Order < orderedGroups[j].Order
			}
			if orderedGroups[i].Title != orderedGroups[j].Title {
				return orderedGroups[i].Title < orderedGroups[j].Title
			}
			return string(orderedGroups[i].Key) < string(orderedGroups[j].Key)
		})
		for _, g := range orderedGroups {
			if len(g.Items) == 1 && len(g.Items[0].Children) > 0 {
				rootItem := g.Items[0]
				header := admdto.AdminMenuItem{
					Key:      g.Key,
					Title:    rootItem.Title,
					Icon:     rootItem.Icon,
					Order:    g.Order,
					Origin:   plugin_mgr.OriginPlugin,
					Visible:  true,
					Slot:     rootItem.Slot,
					Children: rootItem.Children,
				}
				// 组头标题尝试用 menu.<prefix>.title 覆盖（如 menu.base.title）
				if gt := guessPluginGroupTitleFromChildren(header.Children, i18n, locales); gt != "" {
					header.Title = gt
				}
				for i := range header.Children {
					header.Children[i].ParentID = header.Key
				}
				byID[catAppsKey].Children = append(byID[catAppsKey].Children, header)
			} else {
				header := admdto.AdminMenuItem{
					Key:     g.Key,
					Title:   g.Title,
					Order:   g.Order,
					Origin:  plugin_mgr.OriginPlugin,
					Visible: true,
				}
				header.Children = g.Items
				byID[catAppsKey].Children = append(byID[catAppsKey].Children, header)
			}
		}
	}

	for _, cat := range byID {
		if len(cat.Children) > 1 {
			sort.Slice(cat.Children, func(i, j int) bool {
				a, b := cat.Children[i], cat.Children[j]
				if a.Order != b.Order {
					return a.Order < b.Order
				}
				if a.Title != b.Title {
					return a.Title < b.Title
				}
				return a.Key < b.Key
			})
		}
	}

	out := make([]admdto.AdminMenuCategory, 0, len(byID))
	for _, v := range byID {
		if len(v.Children) == 0 {
			continue
		}
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

func addToPluginGroup(groups map[string]*pluginGroup, key, title string, item admdto.AdminMenuItem) {
	g, exists := groups[key]
	if !exists {
		g = &pluginGroup{
			Key:   plugin_mgr.MenuKey(key),
			Title: title,
			Order: item.Order,
		}
		groups[key] = g
	} else if item.Order < g.Order {
		g.Order = item.Order
	}
	child := item
	child.ParentID = plugin_mgr.MenuKey(key)
	g.Items = append(g.Items, child)
}

func formatSlotSeparatorTitle(raw string, i18n []admdto.MenuI18nPackage, locales []string) string {
	raw = strings.TrimSpace(raw)
	sep := " -"
	if raw == "" {
		return i18nOrDefault("menu.section.app", "App", i18n, locales) + sep
	}
	return strings.ToUpper(raw) + sep
}

func formatPluginGroupTitle(raw string, i18n []admdto.MenuI18nPackage, locales []string) string {
	raw = strings.TrimSpace(raw)
	sep := " -"
	if raw == "" {
		return i18nOrDefault("menu.section.plugin", "Plugin", i18n, locales) + sep
	}
	return raw + sep
}

func extractPluginIDFromKey(key plugin_mgr.MenuKey) string {
	const prefix = "plugin:"
	val := string(key)
	if !strings.HasPrefix(val, prefix) {
		return ""
	}
	val = val[len(prefix):]
	if idx := strings.Index(val, ":"); idx >= 0 {
		return val[:idx]
	}
	return ""
}
