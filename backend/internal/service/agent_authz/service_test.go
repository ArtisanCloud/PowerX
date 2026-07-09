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

func TestPermissionCodesFromRecordRequiresStructuredCodes(t *testing.T) {
	record := capmodels.CapabilityRecord{
		ToolScope: datatypes.JSON([]byte(`["base.templates:read","freeform-scope"]`)),
	}
	require.Equal(t, []string{"base.templates:read"}, permissionCodesFromRecord(record))
}
