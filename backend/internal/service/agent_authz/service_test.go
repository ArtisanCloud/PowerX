package agent_authz

import (
	"context"
	"testing"
	"time"

	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	"github.com/google/uuid"
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

func TestPermissionCodesFromRecordDerivesCoreScope(t *testing.T) {
	record := capmodels.CapabilityRecord{
		PluginID:  "corex.platform",
		ToolScope: datatypes.JSON([]byte(`["agent.session"]`)),
	}
	require.Equal(t, []string{"corex.agent.session:use"}, permissionCodesFromRecord(record))
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

func TestAnnotationLocaleTextMap(t *testing.T) {
	record := capmodels.CapabilityRecord{
		Annotations: datatypes.JSON([]byte(`{
			"title_i18n": {
				"zh-CN": "创建模板",
				"en": "Create template",
				"empty": ""
			}
		}`)),
	}

	require.Equal(t, map[string]string{
		"zh-CN": "创建模板",
		"en":    "Create template",
	}, annotationLocaleTextMap(record, "title_i18n"))
}

func TestBaselineGrantAllowed(t *testing.T) {
	ownerPluginID := "com.powerx.plugins.base.local"
	agent := &agentmodel.Agent{OwnerPluginID: &ownerPluginID}

	require.True(t, baselineGrantAllowed(agent, "corex.platform"))
	require.True(t, baselineGrantAllowed(agent, "corex.iam"))
	require.True(t, baselineGrantAllowed(agent, "com.powerx.plugins.base.local"))
	require.False(t, baselineGrantAllowed(agent, "com.powerx.plugins.ecommerce"))
	require.False(t, baselineGrantAllowed(&agentmodel.Agent{}, "com.powerx.plugins.base.local"))
}

func TestModuleFromRecordReadsAnnotation(t *testing.T) {
	record := capmodels.CapabilityRecord{
		PluginID:    "corex.platform",
		Annotations: datatypes.JSON([]byte(`{"module":"iam","permission_code":"corex.iam.member:read"}`)),
	}

	require.Equal(t, "iam", moduleFromRecord(record))
}

func TestIAMRequirementsForCoreRESTCapabilityUsesRESTBinding(t *testing.T) {
	capability := GrantableCapability{
		PluginID:       "corex.platform",
		Module:         "admin",
		PermissionCode: "corex.rest:get_api_v1_admin_agents",
		Protocols: datatypes.JSON([]byte(`[
			{"channel":"rest","method":"GET","endpoint":"/api/v1/admin/agents"}
		]`)),
	}

	require.Equal(t, []iamRequirement{{
		Module:   "admin",
		Resource: "admin_agents",
		Action:   "list",
	}}, iamRequirementsForGrantableCapability(capability))
}

func TestEffectivePermissionsCacheInvalidatesByAgentVersion(t *testing.T) {
	ctx := context.Background()
	svc := &Service{cache: cache.NewMemoryCache(), cacheTTL: time.Minute}
	agentUUID := uuid.New()
	result := EffectivePermissionsResult{
		TenantUUID:         "tenant-a",
		UserUUID:           uuid.NewString(),
		MemberUUID:         uuid.NewString(),
		AgentUUID:          agentUUID,
		AgentAccessAllowed: true,
		Items: []EffectivePermissionItem{{
			CapabilityID:     "capability.one",
			PermissionCode:   "corex.agent:read",
			EffectiveAllowed: true,
		}},
	}

	require.NoError(t, svc.setCachedEffectivePermissions(ctx, "dev", result.TenantUUID, result.UserUUID, result.MemberUUID, 100, false, agentUUID, result))
	cached, ok, err := svc.getCachedEffectivePermissions(ctx, "dev", result.TenantUUID, result.UserUUID, result.MemberUUID, 100, false, agentUUID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, result.Items[0].CapabilityID, cached.Items[0].CapabilityID)

	require.NoError(t, svc.invalidateAgentEffectivePermissionsCache(ctx, "dev", result.TenantUUID, agentUUID))
	_, ok, err = svc.getCachedEffectivePermissions(ctx, "dev", result.TenantUUID, result.UserUUID, result.MemberUUID, 100, false, agentUUID)
	require.NoError(t, err)
	require.False(t, ok)
}
