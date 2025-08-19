package menu

import admdto "github.com/ArtisanCloud/PowerX/api/http/admin/dto"

func BuildSystemMenus() []admdto.AdminMenuItem {
	return []admdto.AdminMenuItem{
		{
			Key:     "agent",
			Title:   "menu.agent",
			Icon:    "i-heroicons-chat-bubble-left-right",
			URL:     "/agent",
			Order:   1,
			Visible: true,
			Origin:  "system",
		},
		{
			Key:     "workflow",
			Title:   "menu.workflow",
			Icon:    "i-heroicons-squares-2x2",
			URL:     "/workflow",
			Order:   2,
			Visible: true,
			Origin:  "system",
		},
		{
			Key:     "plugins",
			Title:   "menu.pluginMarketplace",
			Icon:    "i-heroicons-puzzle-piece",
			URL:     "/plugins",
			Order:   4,
			Visible: true,
			Origin:  "system",
		},
		{
			Key:     "dashboard",
			Title:   "menu.dashboard",
			Icon:    "i-heroicons-home",
			URL:     "/dashboard",
			Order:   3,
			Visible: true,
			Origin:  "system",
		},
		{
			Key:     "settings",
			Title:   "menu.settings",
			Icon:    "i-heroicons-cog-6-tooth",
			Order:   6,
			Visible: true,
			Origin:  "system",
			Children: []admdto.AdminMenuItem{
				{
					Key:      "user-management",
					Title:    "menu.userManagement",
					Icon:     "i-heroicons-user",
					URL:      "/settings/users",
					Order:    1,
					Visible:  true,
					Origin:   "system",
					ParentID: "settings",
				},
				{
					Key:      "role-management",
					Title:    "menu.roleManagement",
					Icon:     "i-heroicons-shield-check",
					URL:      "/settings/roles",
					Order:    2,
					Visible:  true,
					Origin:   "system",
					ParentID: "settings",
				},
				{
					Key:      "system-config",
					Title:    "menu.systemConfig",
					Icon:     "i-heroicons-wrench-screwdriver",
					URL:      "/settings/config",
					Order:    3,
					Visible:  true,
					Origin:   "system",
					ParentID: "settings",
				},
				{
					Key:      "ai-settings",
					Title:    "menu.aiSettings",
					Icon:     "i-heroicons-cpu-chip",
					URL:      "/settings/ai",
					Order:    4,
					Visible:  true,
					Origin:   "system",
					ParentID: "settings",
				},
			},
		},
	}
}
