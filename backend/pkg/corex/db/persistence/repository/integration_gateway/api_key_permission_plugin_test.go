package integration_gateway

import (
	"testing"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	"github.com/stretchr/testify/require"
)

func TestPluginAPIKeyGrantRequiresExactOwnerAndTuple(t *testing.T) {
	grant := models.IntegrationGatewayAPIKeyPermission{Scope: "_scope.plans.read", Action: "read", ResourceType: "capability", ResourcePattern: "plans", PluginID: "com.example.a", Effect: "allow"}
	check := func(item models.IntegrationGatewayAPIKeyPermission, owner string) bool {
		return APIKeyPermissionGrantedForPlugin([]models.IntegrationGatewayAPIKeyPermission{item}, owner, grant.Scope, grant.Action, grant.ResourceType, "plans")
	}
	require.True(t, check(grant, grant.PluginID))
	require.False(t, check(grant, "com.example.b"))
	require.False(t, check(grant, ""))
	for _, owner := range []string{"", "*", "com.example.*"} {
		item := grant
		item.PluginID = owner
		require.False(t, check(item, grant.PluginID))
	}
	for _, field := range []string{"scope", "action", "resource_type", "resource", "effect"} {
		item := grant
		switch field {
		case "scope":
			item.Scope = "_scope.other.read"
		case "action":
			item.Action = "manage"
		case "resource_type":
			item.ResourceType = "api"
		case "resource":
			item.ResourcePattern = "other"
		case "effect":
			item.Effect = "deny"
		}
		require.False(t, check(item, grant.PluginID), field)
	}
	// Core 服务合同仍然按原四元组授权；行上的调用插件 owner 由对应 Core 服务复核。
	require.True(t, APIKeyPermissionGranted([]models.IntegrationGatewayAPIKeyPermission{grant}, grant.Scope, grant.Action, grant.ResourceType, "plans"))
}
