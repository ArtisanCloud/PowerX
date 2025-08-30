package menu

import (
	admdto "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/dto"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

func BuildSystemMenus() []admdto.AdminMenuItem {
	return []admdto.AdminMenuItem{
		{
			Key:     plugin_mgr.KeyAgent,
			Title:   "menu.agent",
			Icon:    "i-heroicons-chat-bubble-left-right",
			URL:     "/agent",
			Order:   1,
			Visible: true,
			Origin:  plugin_mgr.OriginSystem,
			Slot:    plugin_mgr.SlotRoot,
		},
		{
			Key:     plugin_mgr.KeyWorkflow,
			Title:   "menu.workflow",
			Icon:    "i-heroicons-squares-2x2",
			URL:     "/workflow",
			Order:   2,
			Visible: true,
			//Origin:  plugin_mgr.SlotWorkflow,
			Origin: plugin_mgr.OriginSystem,
			Slot:   plugin_mgr.SlotRoot,
		},
		{
			Key:     plugin_mgr.KeyDashboard,
			Title:   "menu.dashboard",
			Icon:    "i-heroicons-home",
			URL:     "/dashboard",
			Order:   3,
			Visible: true,
			Origin:  plugin_mgr.OriginSystem,
		},
		{
			Key:     plugin_mgr.KeyPlugins,
			Title:   "menu.pluginMarketplace",
			Icon:    "i-heroicons-puzzle-piece",
			URL:     "/plugins",
			Order:   4,
			Visible: true,
			Origin:  plugin_mgr.OriginSystem,
		},
		{
			Key:     plugin_mgr.KeySettings,
			Title:   "menu.settings",
			Icon:    "i-heroicons-cog-6-tooth",
			Order:   6,
			Visible: true,
			Origin:  plugin_mgr.OriginSystem,
			Children: []admdto.AdminMenuItem{
				{
					Key:      plugin_mgr.KeyUserManagement,
					Title:    "menu.userManagement",
					Icon:     "i-heroicons-user",
					URL:      "/settings/users",
					Order:    1,
					Visible:  true,
					Origin:   plugin_mgr.OriginSystem,
					ParentID: plugin_mgr.KeySettings,
				},
				{
					Key:      plugin_mgr.KeyRoleManagement,
					Title:    "menu.roleManagement",
					Icon:     "i-heroicons-shield-check",
					URL:      "/settings/roles",
					Order:    2,
					Visible:  true,
					Origin:   plugin_mgr.OriginSystem,
					ParentID: plugin_mgr.KeySettings,
				},
				{
					Key:      plugin_mgr.KeySystemConfig,
					Title:    "menu.systemConfig",
					Icon:     "i-heroicons-wrench-screwdriver",
					URL:      "/settings/config",
					Order:    3,
					Visible:  true,
					Origin:   plugin_mgr.OriginSystem,
					ParentID: plugin_mgr.KeySettings,
				},
				{
					Key:      plugin_mgr.KeyAISettings,
					Title:    "menu.aiSettings",
					Icon:     "i-heroicons-cpu-chip",
					URL:      "/settings/ai",
					Order:    4,
					Visible:  true,
					Origin:   plugin_mgr.OriginSystem,
					ParentID: plugin_mgr.KeySettings,
				},
			},
		},
	}
}
