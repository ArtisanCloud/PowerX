package menu

import "github.com/ArtisanCloud/PowerX/api/http/admin/dto"

func BuildSystemMenus() []dto.AdminMenuItem {
	return []dto.AdminMenuItem{
		{Key: "system:dashboard", Title: "Dashboard", Icon: "LayoutDashboard", URL: "/admin/dashboard", Order: 10, Origin: "system", Permissions: []string{}},
		{Key: "system:users", Title: "Users", Icon: "User", URL: "/admin/users", Order: 20, Origin: "system", Permissions: []string{"iam.user:view"}},
	}
}
