package iamcontract

import (
	"path/filepath"
	"testing"

	admdto "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/dto"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/menu"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestRootPlatformBoundaryHTTPContract(t *testing.T) {
	specPath := filepath.Join(repoRootFromHere(t), "specs", "026-iam", "contracts", "http-openapi.yaml")
	loader := &openapi3.Loader{IsExternalRefsAllowed: true}
	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err, "load 026-iam openapi spec")

	startPath, ok := doc.Paths["/admin/root/support-sessions"]
	require.True(t, ok, "missing /admin/root/support-sessions path")
	require.NotNil(t, startPath.Post, "support session start must be POST")
	require.Contains(t, startPath.Post.Tags, "RootSupport")

	reqSchema := startPath.Post.RequestBody.Value.Content.Get("application/json").Schema
	require.NotNil(t, reqSchema)
	require.Equal(t, "#/components/schemas/RootSupportSessionCreateRequest", reqSchema.Ref)
	require.Contains(t, startPath.Post.Responses, "200")
	require.Contains(t, startPath.Post.Responses, "403", "non-root callers must be rejected explicitly")
	require.Contains(t, startPath.Post.Responses, "404", "unknown target tenants must be rejected explicitly")

	endPath, ok := doc.Paths["/admin/root/support-sessions/{id}/end"]
	require.True(t, ok, "missing /admin/root/support-sessions/{id}/end path")
	require.NotNil(t, endPath.Post, "support session end must be POST")
	require.Contains(t, endPath.Post.Responses, "200")
	require.Contains(t, endPath.Post.Responses, "403")
	require.Contains(t, endPath.Post.Responses, "404")

	createReq := doc.Components.Schemas["RootSupportSessionCreateRequest"]
	require.NotNil(t, createReq, "RootSupportSessionCreateRequest schema missing")
	require.Contains(t, createReq.Value.Required, "target_tenant_uuid")
	require.Contains(t, createReq.Value.Required, "reason")
	require.Equal(t, "uuid", createReq.Value.Properties["target_tenant_uuid"].Value.Format)
	require.ElementsMatch(t, []any{"read_only", "write_enabled"}, createReq.Value.Properties["mode"].Value.Enum)

	session := doc.Components.Schemas["RootSupportSession"]
	require.NotNil(t, session, "RootSupportSession schema missing")
	for _, requiredField := range []string{
		"id",
		"root_user_id",
		"target_tenant_uuid",
		"reason",
		"mode",
		"status",
		"started_at",
		"ended_at",
	} {
		require.Contains(t, session.Value.Properties, requiredField, "RootSupportSession missing %s", requiredField)
	}
}

func TestRootDefaultSystemMenusExcludeTenantBusinessConfiguration(t *testing.T) {
	items := flattenMenuItems(menu.BuildSystemMenus())
	require.NotEmpty(t, items)

	aiSettings := findMenuByPath(items, "/settings/ai")
	require.NotNil(t, aiSettings, "AI Settings menu should remain declared for tenant admin contexts")
	require.Contains(t, aiSettings.Permissions, "admin:tenant")
	require.NotContains(t, aiSettings.Permissions, "admin:root", "root default platform menu must not expose tenant AI Settings")

	tenantBusinessPaths := []string{
		"/settings/ai",
		"/settings/ai/cost",
		"/settings/ai/context-optimizer",
	}
	for _, path := range tenantBusinessPaths {
		item := findMenuByPath(items, path)
		require.NotNil(t, item, "expected menu declaration for %s", path)
		require.Contains(t, item.Permissions, "admin:tenant", "%s must be a tenant-context menu", path)
		require.NotContains(t, item.Permissions, "admin:root", "%s must not be visible in root platform context", path)
	}
}

func flattenMenuItems(items []admdto.AdminMenuItem) []admdto.AdminMenuItem {
	out := make([]admdto.AdminMenuItem, 0, len(items))
	for _, item := range items {
		out = append(out, item)
		if len(item.Children) > 0 {
			out = append(out, flattenMenuItems(item.Children)...)
		}
	}
	return out
}

func findMenuByPath(items []admdto.AdminMenuItem, path string) *admdto.AdminMenuItem {
	for i := range items {
		if items[i].URL == path {
			return &items[i]
		}
	}
	return nil
}
