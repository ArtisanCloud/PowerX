// api/http/admin/plugin/menus_agg.go
package plugin

import (
	mgrimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	admdto "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/dto"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"sort"
	"strings"
)

func BuildPluginMenusPublic(basePrefix string) []admdto.AdminMenuItem {
	mgr := mgrimpl.GetPluginManager()
	list, err := mgr.List(nil)
	if err != nil {
		return nil
	}

	var out []admdto.AdminMenuItem
	for _, p := range list {
		if p.State != "enabled" {
			continue
		}
		if !(p.Frontend.Admin.Kind == "static" && p.Paths.FrontendAdminDir != "") {
			continue
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
			out = append(out, admdto.AdminMenuItem{
				Key:         plugin_mgr.MenuKey("plugin:" + p.ID + ":" + route),
				Title:       m.Title,
				Icon:        m.Icon,
				URL:         url,
				Order:       m.Order,
				Origin:      plugin_mgr.OriginPlugin,
				Visible:     visible, // ✅ 默认可见
				Slot:        slot,    // ✅ 插槽
				Permissions: m.RequiredPolicies,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out
}
