package apikeypermissions

import "testing"

func TestBuildTemplatePermissionsIncludesPluginDebugHostRegister(t *testing.T) {
	rows := BuildTemplatePermissions()
	for _, row := range rows {
		resolved, ok := ResolvePermission(row)
		if !ok {
			continue
		}
		if resolved.Scope == "_scope.plugin.debug_host.register" {
			if resolved.Action != "sync" {
				t.Fatalf("unexpected action: %s", resolved.Action)
			}
			if resolved.ResourceType != "api" {
				t.Fatalf("unexpected resource type: %s", resolved.ResourceType)
			}
			if resolved.ResourcePattern != "POST:/api/v1/internal/plugins/debug-hosts" {
				t.Fatalf("unexpected resource pattern: %s", resolved.ResourcePattern)
			}
			return
		}
	}
	t.Fatal("missing plugin debug host register API key permission")
}
