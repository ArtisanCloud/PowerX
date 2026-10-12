package apikeypermissions

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/stretchr/testify/require"
)

func TestCustomerImportAdminPermissionsAreNotAPIKeyGrants(t *testing.T) {
	require.False(t, DefaultAllowAPIKey(im.Permission{Module: "menu", Resource: "customer", Action: "view"}))
	_, file, _, _ := runtime.Caller(0)
	t.Setenv(platformCapabilitiesDirEnv, filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../config/platform_capabilities")))
	platformPermissionOnce = sync.Once{}
	platformPermissionRows = nil
	platformPermissionErr = nil
	t.Cleanup(func() {
		platformPermissionOnce = sync.Once{}
		platformPermissionRows = nil
		platformPermissionErr = nil
	})
	rows, err := BuildPlatformCapabilityPermissions()
	require.NoError(t, err)
	count := 0
	primaryBindings, repairBindings, profileBindings := 0, 0, 0
	for _, row := range rows {
		var meta map[string]any
		require.NoError(t, json.Unmarshal(row.Meta, &meta))
		if row.Module == "customer" && row.Resource == "admin_customers_accounts_customer_uuid" && row.Action == "update" {
			profileBindings++
			require.Equal(t, "platform_capability", row.Source)
			require.False(t, row.AllowAPIKey)
			require.False(t, DefaultAllowAPIKey(row))
			_, grantable := ResolvePermission(row)
			require.False(t, grantable)
		}
		if meta["capability_id"] == "com.corex.customer.contacts.primary_manage" {
			primaryBindings++
			if strings.HasPrefix(row.Resource, "admin_customers_") {
				require.False(t, row.AllowAPIKey)
			} else {
				require.True(t, row.AllowAPIKey)
				require.Equal(t, true, meta["api_key_explicit"])
				_, grantable := ResolvePermission(row)
				require.True(t, grantable)
			}
		}
		if meta["capability_id"] == "com.corex.customer.contacts.admin_repair" {
			repairBindings++
			require.False(t, row.AllowAPIKey)
			require.False(t, DefaultAllowAPIKey(row))
		}
		if meta["capability_id"] != "com.corex.customer.accounts.admin_import" {
			continue
		}
		count++
		require.Equal(t, "customer", row.Module)
		require.True(t, strings.HasPrefix(row.Resource, "admin_customers_imports"))
		require.False(t, row.AllowAPIKey)
		require.False(t, DefaultAllowAPIKey(row))
		_, grantable := ResolvePermission(row)
		require.False(t, grantable)
	}
	require.Equal(t, 3, count)
	require.Equal(t, 1, profileBindings)
	require.Equal(t, 2, primaryBindings)
	require.Equal(t, 3, repairBindings)
}
