package iamcontract

import (
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestIAMMeContextHTTPContract(t *testing.T) {
	specPath := filepath.Join(repoRootFromHere(t), "specs", "026-iam", "contracts", "http-openapi.yaml")
	loader := &openapi3.Loader{IsExternalRefsAllowed: true}
	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err, "load 026-iam openapi spec")

	_, ok := doc.Paths["/admin/user/auth/me/context"]
	require.True(t, ok, "missing /admin/user/auth/me/context path")
	_, ok = doc.Paths["/admin/user/auth/me/tenants"]
	require.True(t, ok, "missing /admin/user/auth/me/tenants path")

	meCtx := doc.Components.Schemas["MeContext"]
	require.NotNil(t, meCtx, "MeContext schema missing")
	require.Contains(t, meCtx.Value.Required, "is_root")
	require.Contains(t, meCtx.Value.Required, "members")

	currentTenant := meCtx.Value.Properties["current_tenant_uuid"]
	require.NotNil(t, currentTenant)
	require.Equal(t, "string", currentTenant.Value.Type)
	require.Equal(t, "uuid", currentTenant.Value.Format)

	memberItem := meCtx.Value.Properties["members"]
	require.NotNil(t, memberItem)
	require.NotNil(t, memberItem.Value.Items)
	require.Equal(t, "#/components/schemas/MemberTenant", memberItem.Value.Items.Ref)
}
