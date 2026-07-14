package iamcontract

import (
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestIAMRBACBoundaryHTTPContract(t *testing.T) {
	specPath := filepath.Join(repoRootFromHere(t), "specs", "026-iam", "contracts", "http-openapi.yaml")
	loader := &openapi3.Loader{IsExternalRefsAllowed: true}
	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err, "load 026-iam openapi spec")

	switchPath, ok := doc.Paths["/admin/user/auth/me/switch-tenant"]
	require.True(t, ok, "missing /admin/user/auth/me/switch-tenant path")
	require.NotNil(t, switchPath.Post)

	reqSchema := switchPath.Post.RequestBody.Value.Content.Get("application/json").Schema
	require.NotNil(t, reqSchema)
	require.Contains(t, reqSchema.Value.Required, "tenant_uuid")
	tenantUUID := reqSchema.Value.Properties["tenant_uuid"]
	require.NotNil(t, tenantUUID)
	require.Equal(t, "string", tenantUUID.Value.Type)
	require.Equal(t, "uuid", tenantUUID.Value.Format)

	_, ok = switchPath.Post.Responses["200"]
	require.True(t, ok, "switch-tenant missing 200 response")
	_, ok = switchPath.Post.Responses["403"]
	require.True(t, ok, "switch-tenant missing 403 response")

	checkPath, ok := doc.Paths["/admin/iam/me/check"]
	require.True(t, ok, "missing /admin/iam/me/check path")
	require.NotNil(t, checkPath.Get)
	require.Len(t, checkPath.Get.Parameters, 4)

	seen := map[string]bool{}
	for _, p := range checkPath.Get.Parameters {
		if p == nil || p.Value == nil {
			continue
		}
		if p.Value.Name == "permission" || p.Value.Name == "module" || p.Value.Name == "action" || p.Value.Name == "resource" {
			require.False(t, p.Value.Required)
			require.Equal(t, "query", p.Value.In)
			seen[p.Value.Name] = true
		}
	}
	require.True(t, seen["permission"], "missing query param permission")
	require.True(t, seen["module"], "missing query param module")
	require.True(t, seen["action"], "missing required query param action")
	require.True(t, seen["resource"], "missing required query param resource")
}
