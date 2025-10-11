// api/http/admin/plugin/menus_agg.go
package plugin

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

	mgrimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	admdto "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/dto"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

type PluginMenusPublic struct {
	Items []admdto.AdminMenuItem
	I18n  []admdto.MenuI18nPackage
}

func BuildPluginMenusPublic(ctx context.Context, basePrefix string, locales []string) PluginMenusPublic {
	mgr := mgrimpl.GetPluginManager()
	list, err := mgr.List(ctx)
	if err != nil {
		return PluginMenusPublic{}
	}

	// ★ 新增：总览
	log.Printf("[menu-builder] registry=%d", len(list))

	preferredLocales := normalizeLocalePreference(locales)
	out := PluginMenusPublic{Items: make([]admdto.AdminMenuItem, 0, len(list))}
	for _, p := range list {
		// ★ 新增：逐插件观测
		log.Printf("[menu-builder] id=%s state=%s kind=%s adminDir=%q menus=%d",
			p.ID, p.State, p.Frontend.Admin.Kind, p.Paths.FrontendAdminDir, len(p.Frontend.Admin.Menus))

		if p.State == plugin_mgr.StateDisabled {
			log.Printf("[menu-builder] skip=%s reason=state=disabled", p.ID)
			continue
		}
		if p.State != plugin_mgr.StateEnabled {
			log.Printf("[menu-builder] include=%s state=%s (menus only)", p.ID, p.State)
		}

		// 只要插件声明了菜单，就接入（无论 static / process / proxy）
		if len(p.Frontend.Admin.Menus) == 0 {
			log.Printf("[menu-builder] skip=%s reason=no-menus", p.ID)
			continue
		}

		// i18n：仅当有静态目录时再去加载（process/proxy 没静态包也不影响菜单展示）
		var bundle *admdto.MenuI18nPackage // 按你原来的类型
		if p.Paths.FrontendAdminDir != "" {
			b := loadPluginMenuI18n(ctx, p, preferredLocales)
			if b != nil {
				bundle = b
				out.I18n = append(out.I18n, *b)
			}
		}

		root := basePrefix + "/" + p.ID + "/admin/"
		for _, m := range p.Frontend.Admin.Menus {
			item := convertPluginMenuItem(p.ID, root, "", m, preferredLocales, bundle)
			if item.Slot == "" {
				item.Slot = plugin_mgr.SlotPlugins
			}
			out.Items = append(out.Items, item)
		}
	}
	sort.Slice(out.Items, func(i, j int) bool { return out.Items[i].Order < out.Items[j].Order })
	return out
}

func convertPluginMenuItem(pluginID, root string, parent plugin_mgr.MenuKey, m plugin_mgr.MenuItem, locales []string, bundle *admdto.MenuI18nPackage) admdto.AdminMenuItem {
	route := strings.TrimLeft(m.Route, "/")
	url := root
	if route != "" {
		url = root + route
	}

	visible := true
	if m.Visible != nil {
		visible = *m.Visible
	}
	slot := m.Slot

	title := resolveMenuTitle(m, locales, bundle)
	titleI18n := toMenuI18nLabel(m.TitleI18n, title)

	key := makePluginMenuKey(pluginID, m, route)
	item := admdto.AdminMenuItem{
		Key:         key,
		Title:       title,
		Icon:        m.Icon,
		URL:         url,
		Order:       m.Order,
		Origin:      plugin_mgr.OriginPlugin,
		Visible:     visible,
		Slot:        slot,
		Permissions: m.RequiredPolicies,
		TitleI18n:   titleI18n,
	}
	if parent != "" {
		item.ParentID = parent
	}

	if len(m.Children) > 0 {
		item.Children = make([]admdto.AdminMenuItem, 0, len(m.Children))
		for _, child := range m.Children {
			childItem := convertPluginMenuItem(pluginID, root, key, child, locales, bundle)
			item.Children = append(item.Children, childItem)
		}
	}
	return item
}

func makePluginMenuKey(pluginID string, m plugin_mgr.MenuItem, route string) plugin_mgr.MenuKey {
	segment := strings.TrimSpace(m.ID)
	if segment == "" {
		segment = strings.Trim(route, "/")
	}
	if segment == "" {
		segment = strings.Trim(strings.TrimSpace(m.Path), "/")
	}
	if segment == "" {
		segment = fmt.Sprintf("auto-%d", m.Order)
	}
	return plugin_mgr.MenuKey("plugin:" + pluginID + ":" + segment)
}

func toMenuI18nLabel(label *plugin_mgr.MenuLabel, fallback string) *admdto.MenuI18nLabel {
	if label == nil {
		return nil
	}
	out := &admdto.MenuI18nLabel{
		Namespace: label.Namespace,
		Key:       label.Key,
		Default:   label.Default,
	}
	if strings.TrimSpace(out.Default) == "" && strings.TrimSpace(fallback) != "" {
		out.Default = fallback
	}
	return out
}

func resolveMenuTitle(item plugin_mgr.MenuItem, locales []string, bundle *admdto.MenuI18nPackage) string {
	if bundle != nil && item.TitleI18n != nil {
		if txt, ok := translateLabel(bundle, locales, item.TitleI18n); ok && strings.TrimSpace(txt) != "" {
			return txt
		}
	}
	if strings.TrimSpace(item.Title) != "" {
		return item.Title
	}
	if item.TitleI18n != nil {
		if item.TitleI18n.Default != "" {
			return item.TitleI18n.Default
		}
		return item.TitleI18n.Key
	}
	return ""
}

func translateLabel(bundle *admdto.MenuI18nPackage, locales []string, label *plugin_mgr.MenuLabel) (string, bool) {
	if bundle == nil || label == nil {
		return "", false
	}
	ns := strings.TrimSpace(label.Namespace)
	if ns == "" {
		ns = strings.TrimSpace(bundle.DefaultNamespace)
	}
	if ns == "" {
		return "", false
	}
	keys := strings.Split(label.Key, ".")
	if len(keys) == 0 {
		return "", false
	}
	ordered := locales
	if len(ordered) == 0 {
		ordered = make([]string, 0, len(bundle.Locales))
		for loc := range bundle.Locales {
			ordered = append(ordered, loc)
		}
	}
	for _, loc := range ordered {
		loc = strings.TrimSpace(loc)
		if loc == "" {
			continue
		}
		nsMap, ok := bundle.Locales[loc]
		if !ok {
			continue
		}
		root, ok := nsMap[ns]
		if !ok {
			continue
		}
		if txt, ok := findNestedString(root, keys); ok {
			return txt, true
		}
	}
	return "", false
}

func findNestedString(root map[string]any, path []string) (string, bool) {
	current := any(root)
	for i, segment := range path {
		m, ok := current.(map[string]any)
		if !ok {
			return "", false
		}
		value, exists := m[segment]
		if !exists {
			return "", false
		}
		if i == len(path)-1 {
			switch v := value.(type) {
			case string:
				return v, true
			case fmt.Stringer:
				return v.String(), true
			case []byte:
				return string(v), true
			default:
				return fmt.Sprint(v), true
			}
		}
		current = value
	}
	return "", false
}

func normalizeLocalePreference(locales []string) []string {
	if len(locales) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(locales)*2)
	ordered := make([]string, 0, len(locales)*2)
	appendLocale := func(loc string) {
		loc = strings.TrimSpace(loc)
		if loc == "" {
			return
		}
		if _, exists := seen[loc]; exists {
			return
		}
		seen[loc] = struct{}{}
		ordered = append(ordered, loc)
	}
	for _, loc := range locales {
		appendLocale(loc)
		if idx := strings.IndexAny(loc, "-_"); idx > 0 {
			appendLocale(loc[:idx])
		}
	}
	return ordered
}
