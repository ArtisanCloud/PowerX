package agent_authz

import (
	"testing"

	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestParsePermissionCode(t *testing.T) {
	module, resource, action, ok := ParsePermissionCode("base.templates:create")
	require.True(t, ok)
	require.Equal(t, "base", module)
	require.Equal(t, "templates", resource)
	require.Equal(t, "create", action)

	_, _, _, ok = ParsePermissionCode("base.templates")
	require.False(t, ok)
}

func TestParsePermissionCodeForPluginUsesPluginIDBoundary(t *testing.T) {
	module, resource, action, ok := ParsePermissionCodeForPlugin("com.powerx.plugins.base.template:create", "com.powerx.plugins.base")
	require.True(t, ok)
	require.Equal(t, "com.powerx.plugins.base", module)
	require.Equal(t, "template", resource)
	require.Equal(t, "create", action)

	module, resource, action, ok = ParsePermissionCodeForPlugin("com.powerx.plugins.ecommerce.product.category:read", "com.powerx.plugins.ecommerce")
	require.True(t, ok)
	require.Equal(t, "com.powerx.plugins.ecommerce", module)
	require.Equal(t, "product.category", resource)
	require.Equal(t, "read", action)
}

func TestPermissionCodesFromRecordRequiresStructuredCodes(t *testing.T) {
	record := capmodels.CapabilityRecord{
		PluginID:  "base",
		ToolScope: datatypes.JSON([]byte(`["base.templates:read","freeform-scope"]`)),
	}
	require.Equal(t, []string{"base.templates:read"}, permissionCodesFromRecord(record))
}

func TestPermissionCodesFromRecordReadsExplicitAnnotation(t *testing.T) {
	record := capmodels.CapabilityRecord{
		PluginID:    "base",
		ToolScope:   datatypes.JSON([]byte(`["freeform-scope"]`)),
		Annotations: datatypes.JSON([]byte(`{"permission_code":"base.templates:create"}`)),
	}
	require.Equal(t, []string{"base.templates:create"}, permissionCodesFromRecord(record))
}

func TestPermissionCodesFromRecordReadsDottedPluginAnnotation(t *testing.T) {
	record := capmodels.CapabilityRecord{
		PluginID:    "com.powerx.plugins.base",
		Annotations: datatypes.JSON([]byte(`{"permission_codes":["com.powerx.plugins.base.template:create"]}`)),
	}
	require.Equal(t, []string{"com.powerx.plugins.base.template:create"}, permissionCodesFromRecord(record))
}
