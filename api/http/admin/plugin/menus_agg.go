package plugin

import (
	admdto "github.com/ArtisanCloud/PowerX/api/http/admin/dto"
	mgrimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
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
			out = append(out, admdto.AdminMenuItem{
				Key:         "plugin:" + p.ID + ":" + route,
				Title:       m.Title,
				Icon:        m.Icon,
				URL:         url,
				Order:       m.Order,
				Origin:      "plugin",
				Permissions: m.RequiredPolicies,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out
}
