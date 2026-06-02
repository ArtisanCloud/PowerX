package iamcontract

import (
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestIAMMigrationReportHTTPContract(t *testing.T) {
	specPath := filepath.Join(repoRootFromHere(t), "specs", "026-iam", "contracts", "http-openapi.yaml")
	loader := &openapi3.Loader{IsExternalRefsAllowed: true}
	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err, "load 026-iam openapi spec")

	reportPath, ok := doc.Paths["/admin/iam/migration/report"]
	require.True(t, ok, "missing IAM migration report path")
	require.NotNil(t, reportPath.Get, "migration report must be GET")
	require.Equal(t, "getIAMMigrationReport", reportPath.Get.OperationID)
	require.Contains(t, reportPath.Get.Tags, "IAMMigration")
	require.Contains(t, reportPath.Get.Responses, "200")

	fixPath, ok := doc.Paths["/admin/iam/migration/fix-owner"]
	require.True(t, ok, "missing IAM migration owner fix path")
	require.NotNil(t, fixPath.Post, "owner auto-fix must be POST")
	require.Equal(t, "fixMissingTenantOwner", fixPath.Post.OperationID)
	require.Contains(t, fixPath.Post.Tags, "IAMMigration")
	require.Contains(t, fixPath.Post.Responses, "200")
	require.Contains(t, fixPath.Post.Responses, "403", "non-root callers must be rejected explicitly")

	report := doc.Components.Schemas["IAMMigrationReport"]
	require.NotNil(t, report, "IAMMigrationReport schema missing")
	for _, field := range []string{
		"root_users",
		"system_tenant_status",
		"root_system_member_status",
		"tenant_owner_missing",
		"tenant_admin_missing",
		"auto_fix_candidates",
		"manual_fix_required",
	} {
		require.Contains(t, report.Value.Properties, field, "IAMMigrationReport missing %s", field)
	}

	fix := doc.Components.Schemas["IAMMigrationFixOwnerResponse"]
	require.NotNil(t, fix, "IAMMigrationFixOwnerResponse schema missing")
	data := fix.Value.Properties["data"]
	require.NotNil(t, data, "IAMMigrationFixOwnerResponse.data missing")
	require.Contains(t, data.Value.Properties, "fixed_tenant_uuids")
	require.Contains(t, data.Value.Properties, "report")
}
