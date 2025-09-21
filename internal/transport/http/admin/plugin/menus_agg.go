// api/http/admin/plugin/menus_agg.go
package plugin

import (
	"context"
	"fmt"
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

	preferredLocales := normalizeLocalePreference(locales)
	out := PluginMenusPublic{Items: make([]admdto.AdminMenuItem, 0, len(list))}
	for _, p := range list {
		if p.State != plugin_mgr.StateEnabled {
			continue
		}
		if !(p.Frontend.Admin.Kind == plugin_mgr.FrontendKindStatic && p.Paths.FrontendAdminDir != "") {
			continue
		}

		bundle := loadPluginMenuI18n(ctx, p, preferredLocales)
		if bundle != nil {
			out.I18n = append(out.I18n, *bundle)
		}

		root := basePrefix + "/" + p.ID + "/admin/"
		for _, m := range p.Frontend.Admin.Menus {
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
			if slot == "" {
				slot = plugin_mgr.SlotPlugins
			}

			title := resolveMenuTitle(m, preferredLocales, bundle)
			var titleI18n *admdto.MenuI18nLabel
			if m.TitleI18n != nil {
				titleI18n = &admdto.MenuI18nLabel{
					Namespace: m.TitleI18n.Namespace,
					Key:       m.TitleI18n.Key,
					Default:   m.TitleI18n.Default,
				}
				if titleI18n.Default == "" && title != "" {
					titleI18n.Default = title
				}
			}

			out.Items = append(out.Items, admdto.AdminMenuItem{
				Key:         plugin_mgr.MenuKey("plugin:" + p.ID + ":" + route),
				Title:       title,
				Icon:        m.Icon,
				URL:         url,
				Order:       m.Order,
				Origin:      plugin_mgr.OriginPlugin,
				Visible:     visible, // ✅ 默认可见
				Slot:        slot,    // ✅ 插槽
				Permissions: m.RequiredPolicies,
				TitleI18n:   titleI18n,
			})
		}
	}
	sort.Slice(out.Items, func(i, j int) bool { return out.Items[i].Order < out.Items[j].Order })
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
