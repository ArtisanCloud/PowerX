package menu

import (
	"testing"

	admdto "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/dto"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

func TestSplitPolicyTriple(t *testing.T) {
	module, resource, action := splitPolicyTriple("menu:agent.chat:read")
	if module != "menu" || resource != "agent.chat" || action != "read" {
		t.Fatalf("unexpected triple: %s %s %s", module, resource, action)
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
			Permissions: []string{"menu:settings:read"},
			Children: []admdto.AdminMenuItem{
				{
					Key:         "settings_users",
					Title:       "Users",
					URL:         "/settings/users",
					Permissions: []string{"menu:settings.users:read"},
				},
			},
		},
	}

	filtered := filterMenusByPermission(items, func(perms []string) bool {
		for _, perm := range perms {
			if perm == "menu:settings.users:read" {
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
		"agent_chat":        "/agent/sessions",
		"agent_management":  "/settings/ai/agents",
		"agent_team":        "/settings/ai/agent-teams",
		"agent_team_tasks":  "/agent/team-tasks",
		"agent_traces":      "/agent/traces",
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
