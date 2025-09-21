// api/http/admin/plugin/menus_agg.go
package plugin

import (
	"context"
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

func BuildPluginMenusPublic(ctx context.Context, basePrefix string) PluginMenusPublic {
	mgr := mgrimpl.GetPluginManager()
	list, err := mgr.List(ctx)
	if err != nil {
		return PluginMenusPublic{}
	}

	out := PluginMenusPublic{Items: make([]admdto.AdminMenuItem, 0, len(list))}
	for _, p := range list {
		if p.State != plugin_mgr.StateEnabled {
			continue
		}
		if !(p.Frontend.Admin.Kind == plugin_mgr.FrontendKindStatic && p.Paths.FrontendAdminDir != "") {
			continue
		}

		if bundle := loadPluginMenuI18n(ctx, p); bundle != nil {
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

			title := m.Title
			var titleI18n *admdto.MenuI18nLabel
			if m.TitleI18n != nil {
				if def := m.TitleI18n.Default; def != "" {
					title = def
				} else if title == "" {
					title = m.TitleI18n.Key
				}
				titleI18n = &admdto.MenuI18nLabel{
					Namespace: m.TitleI18n.Namespace,
					Key:       m.TitleI18n.Key,
				}
				if title != "" {
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
