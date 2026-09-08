package mediacontract

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTenantMediaHostContractLocks the only plugin-facing Media surface to
// UUID resources and Core-issued transfer tickets. Legacy admin/storage DTOs
// are deliberately outside this Host Contract.
func TestTenantMediaHostContractIsUUIDOnly(t *testing.T) {
	root := tenantMediaRepoRoot(t)
	spec, err := os.ReadFile(filepath.Join(root, "specs", "001-media-storage", "contracts", "http-openapi.yaml"))
	require.NoError(t, err)
	content := string(spec)

	for _, required := range []string{
		"/tenant/media/assets/{asset_uuid}",
		"/tenant/media/assets/variants/{variant_uuid}",
		"/tenant/media/assets/{asset_uuid}/presign-upload",
		"/tenant/media/assets/{asset_uuid}/complete-upload",
		"/tenant/media/assets/{asset_uuid}/presign-download",
		"MEDIA_UNAUTHORIZED",
		"MEDIA_FORBIDDEN",
		"MEDIA_ASSET_NOT_FOUND",
		"MEDIA_UPLOAD_VALIDATION_FAILED",
	} {
		require.Contains(t, content, required)
	}
	for _, forbidden := range []string{
		"objectKey", "object_key", "ownerSubjectId", "tenantId",
		"/media/assets/{uuid}/variants/{variant}",
	} {
		require.NotContains(t, content, forbidden)
	}

	capabilities, err := os.ReadFile(filepath.Join(root, "backend", "config", "platform_capabilities", "media.yaml"))
	require.NoError(t, err)
	capabilityContract := string(capabilities)
	require.NotContains(t, capabilityContract, "/api/v1/media/assets/{uuid}/variants/{variant}")
	require.NotContains(t, capabilityContract, "powerx.media.v1.MediaAssetAdminService")
}

func tenantMediaRepoRoot(t testing.TB) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", "..", ".."))
}
