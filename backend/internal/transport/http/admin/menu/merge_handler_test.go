package menu

import (
	"testing"

	admdto "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/dto"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

func TestSplitPolicyTriple(t *testing.T) {
	module, resource, action := splitPolicyTriple("menu.agent.chat:view")
	if module != "menu.agent" || resource != "chat" || action != "view" {
		t.Fatalf("unexpected triple: %s %s %s", module, resource, action)
	}

	module, resource, action = splitPolicyTriple("menu.ai_craft:view")
	if module != "menu" || resource != "ai_craft" || action != "view" {
		t.Fatalf("unexpected dotted permission code: %s %s %s", module, resource, action)
	}

	module, resource, action = splitPolicyTriple("production.sample_track:read")
	if module != "production" || resource != "sample_track" || action != "read" {
		t.Fatalf("unexpected business permission code: %s %s %s", module, resource, action)
	}

	module, resource, action = splitPolicyTriple("menu.operations.tracking_orders.sample_tracks:view")
	if module != "menu.operations.tracking_orders" || resource != "sample_tracks" || action != "view" {
		t.Fatalf("unexpected nested plugin menu permission code: %s %s %s", module, resource, action)
	}

	module, resource, action = splitPolicyTriple("admin:root")
	if module != "admin" || resource != "root" || action != "view" {
		t.Fatalf("unexpected admin policy: %s %s %s", module, resource, action)
	}

	module, resource, action = splitPolicyTriple("invalid:too:many:parts")
	if module != "" || resource != "" || action != "" {
		t.Fatalf("invalid policy should return empty triple: %s %s %s", module, resource, action)
	}
}

func TestFilterMenusByPermissionKeepsParentWhenChildAllowed(t *testing.T) {
	items := []admdto.AdminMenuItem{
		{
			Key:         "settings",
			Title:       "Settings",
			Permissions: []string{"menu.settings:view"},
			Children: []admdto.AdminMenuItem{
				{
					Key:         "settings_users",
					Title:       "Users",
					URL:         "/settings/users",
					Permissions: []string{"menu.settings.users:view"},
				},
			},
		},
	}

	filtered := filterMenusByPermission(items, func(perms []string) bool {
		for _, perm := range perms {
			if perm == "menu.settings.users:view" {
				return true
			}
		}
		return false
	})

	if len(filtered) != 1 {
		t.Fatalf("expected parent to be kept, got %d", len(filtered))
	}
	if filtered[0].Key != plugin_mgr.MenuKey("settings") {
		t.Fatalf("unexpected parent key: %s", filtered[0].Key)
	}
	if len(filtered[0].Children) != 1 || filtered[0].Children[0].Key != plugin_mgr.MenuKey("settings_users") {
		t.Fatalf("expected allowed child to be kept: %+v", filtered[0].Children)
	}
}

func TestGroupAsCategoriesKeepsFilteredPluginMenuInApps(t *testing.T) {
	items := []admdto.AdminMenuItem{
		{
			Key:         "plugin:com.powerx.plugins.ai-craft:ai_craft",
			Title:       "AI Craft",
			Icon:        "i-heroicons-sparkles",
			URL:         "/_p/com.powerx.plugins.ai-craft/admin/operations/ai-craft/sessions",
			Order:       20,
			Origin:      plugin_mgr.OriginPlugin,
			Visible:     true,
			Slot:        plugin_mgr.SlotPlugins,
			Permissions: []string{"menu.ai_craft:view"},
			Children: []admdto.AdminMenuItem{
				{
					Key:         "plugin:com.powerx.plugins.ai-craft:sessions",
					Title:       "Session Center",
					URL:         "/_p/com.powerx.plugins.ai-craft/admin/operations/ai-craft/sessions",
					Origin:      plugin_mgr.OriginPlugin,
					Visible:     true,
					Permissions: []string{"menu.operations.sessions:view"},
				},
			},
		},
	}

	filtered := filterMenusByPermission(items, func(perms []string) bool {
		for _, perm := range perms {
			module, resource, action := splitPolicyTriple(perm)
			if module == "menu.operations" && resource == "sessions" && action == "view" {
				return true
			}
		}
		return false
	})
	categories := groupAsCategories(filtered, nil, nil)

	var apps *admdto.AdminMenuCategory
	for i := range categories {
		if categories[i].ID == plugin_mgr.MenuKey("cat:market") {
			apps = &categories[i]
			break
		}
	}
	if apps == nil {
		t.Fatalf("expected plugin menu category")
	}
	if len(apps.Children) != 1 {
		t.Fatalf("expected one plugin group, got %+v", apps.Children)
	}
	if apps.Children[0].Title != "AI Craft" {
		t.Fatalf("expected plugin title, got %q", apps.Children[0].Title)
	}
	if len(apps.Children[0].Children) != 1 || apps.Children[0].Children[0].Key != plugin_mgr.MenuKey("plugin:com.powerx.plugins.ai-craft:sessions") {
		t.Fatalf("expected filtered plugin child in apps category: %+v", apps.Children)
	}
}

func TestPluginSystemMenuChildrenUseDistinctPaths(t *testing.T) {
	var marketPath string
	var subscriptionsPath string
	for _, item := range BuildSystemMenus() {
		if item.Key != plugin_mgr.KeyPlugins {
			continue
		}
		for _, child := range item.Children {
			switch child.Key {
			case plugin_mgr.MenuKey("plugin_market"):
				marketPath = child.URL
			case plugin_mgr.MenuKey("plugin_subscriptions"):
				subscriptionsPath = child.URL
			}
		}
	}

	if marketPath != "/plugins/market" {
		t.Fatalf("unexpected plugin market path: %s", marketPath)
	}
	if subscriptionsPath != "/plugins/installed" {
		t.Fatalf("unexpected plugin subscriptions path: %s", subscriptionsPath)
	}
	if marketPath == subscriptionsPath {
		t.Fatalf("plugin market and subscriptions must not share one path")
	}
}

func TestAgentSystemMenuContainsWorkspaceChildren(t *testing.T) {
	want := map[plugin_mgr.MenuKey]string{
		"agent_chat":       "/agent/sessions",
		"agent_management": "/settings/ai/agents",
		"agent_team":       "/settings/ai/agent-teams",
		"agent_team_tasks": "/agent/team-tasks",
		"agent_traces":     "/agent/traces",
	}

	got := map[plugin_mgr.MenuKey]string{}
	for _, item := range BuildSystemMenus() {
		if item.Key != plugin_mgr.KeyAgent {
			continue
		}
		for _, child := range item.Children {
			got[child.Key] = child.URL
		}
	}

	for key, path := range want {
		if got[key] != path {
			t.Fatalf("agent child %s path mismatch: got %q want %q", key, got[key], path)
		}
	}
}

func TestSettingsSystemMenuContainsGovernanceEntries(t *testing.T) {
	want := map[plugin_mgr.MenuKey]string{
		"metadata_governance":  "/settings/metadata-governance",
		"integration_api_keys": "/settings/integration-api-keys",
		"open_capabilities":    "/settings/open-capabilities",
		"event_fabric":         "/settings/event-fabric",
	}

	got := map[plugin_mgr.MenuKey]string{}
	for _, item := range BuildSystemMenus() {
		if item.Key != plugin_mgr.KeySettings {
			continue
		}
		for _, child := range item.Children {
			got[child.Key] = child.URL
		}
	}

	for key, path := range want {
		if got[key] != path {
			t.Fatalf("settings child %s path mismatch: got %q want %q", key, got[key], path)
		}
	}
}
