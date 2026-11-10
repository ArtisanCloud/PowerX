package capability_registry

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	iammodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	coreiam "github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
)

func ownedAPIKeyDeclaration(pluginID, capabilityID string) pluginPermissionDeclaration {
	return pluginPermissionDeclaration{
		Type: "api", PermissionCode: "commerce.plans_service:read", Module: "commerce", Resource: "plans_service", Action: "read", Independent: true,
		TitleI18n: map[string]string{"zh-CN": "读取套餐"}, DescriptionI18n: map[string]string{"zh-CN": "读取本租户的套餐"}, RiskLevel: "low", DataScope: "tenant",
		ProtocolBindings: []permissionProtocolBinding{{Channel: "rest", Method: "POST", Path: "/plugin/plans/query", ActorContext: "service_actor", ResourceScope: "tenant"}},
		APIKey:           &plugin_mgr.APIKeyPermissionSpec{CapabilityID: capabilityID, PluginID: pluginID, Scope: "_scope.plans.read", Action: "read", ResourceType: "capability", ResourcePattern: "plans", Effect: "allow"},
	}
}

func TestExplicitPluginPermissionWinsInBothCatalogOrders(t *testing.T) {
	for _, explicitFirst := range []bool{true, false} {
		name := "inferred_first"
		if explicitFirst {
			name = "explicit_first"
		}
		t.Run(name, func(t *testing.T) {
			const pluginID = "com.example.plans"
			db := newMemoryDB(t)
			w := NewSyncWorker(SyncWorkerConfig{DB: db})
			decl := ownedAPIKeyDeclaration(pluginID, pluginID+".read")
			explicit := catalogCapability{ID: pluginID + ".read", ToolScope: []string{decl.PermissionCode}, Permissions: []pluginPermissionDeclaration{decl}}
			inferred := catalogCapability{ID: pluginID + ".export", ToolScope: []string{decl.PermissionCode}}
			capabilities := []catalogCapability{explicit, inferred}
			if !explicitFirst {
				capabilities[0], capabilities[1] = capabilities[1], capabilities[0]
			}
			require.NoError(t, validateCatalogAPIKeyPermissionOwnership(pluginMetadata{ID: pluginID}, capabilities))
			for repeat := 0; repeat < 2; repeat++ {
				for _, capability := range capabilities {
					require.NoError(t, w.syncCapabilityPermissions(context.Background(), pluginMetadata{ID: pluginID}, capability))
				}
			}
			var row iammodel.Permission
			require.NoError(t, db.Where("module=? AND resource=? AND action=?", "commerce", "plans_service", "read").First(&row).Error)
			require.True(t, row.AllowAPIKey)
			var meta map[string]any
			require.NoError(t, json.Unmarshal(row.Meta, &meta))
			require.Equal(t, true, meta["api_key_explicit"])
			require.Equal(t, explicit.ID, meta["capability_id"])
			// 原 capability 显式删除 api_key 时仍然及时收紧，不能永久保留旧授权。
			explicit.Permissions[0].APIKey = nil
			require.NoError(t, w.syncCapabilityPermissions(context.Background(), pluginMetadata{ID: pluginID}, explicit))
			require.NoError(t, db.Where("id=?", row.ID).First(&row).Error)
			require.False(t, row.AllowAPIKey)
			require.NoError(t, json.Unmarshal(row.Meta, &meta))
			require.NotContains(t, string(row.Meta), "api_key_explicit")
		})
	}
}

func TestPluginAPIKeyConflictingDeclarationsFailBeforeReplacement(t *testing.T) {
	const pluginID = "com.example.plans"
	a := catalogCapability{ID: pluginID + ".read", Permissions: []pluginPermissionDeclaration{ownedAPIKeyDeclaration(pluginID, pluginID+".read")}}
	b := catalogCapability{ID: pluginID + ".export", Permissions: []pluginPermissionDeclaration{ownedAPIKeyDeclaration(pluginID, pluginID+".export")}}
	for _, keyed := range []bool{true, false} {
		second := b
		second.Permissions = append([]pluginPermissionDeclaration(nil), b.Permissions...)
		if !keyed {
			second.Permissions[0].APIKey = nil
		}
		require.ErrorIs(t, validateCatalogAPIKeyPermissionOwnership(pluginMetadata{ID: pluginID}, []catalogCapability{a, second}), iamrepo.ErrPluginAPIKeyPermissionConflict)
	}
	db := newMemoryDB(t)
	w := NewSyncWorker(SyncWorkerConfig{DB: db})
	require.NoError(t, w.syncCapabilityPermissions(context.Background(), pluginMetadata{ID: pluginID}, a))
	require.ErrorIs(t, w.syncCapabilityPermissions(context.Background(), pluginMetadata{ID: pluginID}, b), iamrepo.ErrPluginAPIKeyPermissionConflict)
	otherPlugin := "com.example.other"
	b.ID = otherPlugin + ".read"
	b.Permissions = []pluginPermissionDeclaration{ownedAPIKeyDeclaration(otherPlugin, b.ID)}
	require.ErrorIs(t, w.syncCapabilityPermissions(context.Background(), pluginMetadata{ID: otherPlugin}, b), iamrepo.ErrPluginAPIKeyPermissionConflict)
	var row iammodel.Permission
	require.NoError(t, db.Where("module=? AND resource=? AND action=?", "commerce", "plans_service", "read").First(&row).Error)
	require.True(t, row.AllowAPIKey)
	require.Equal(t, "plugin:"+pluginID, row.Source)
	var meta map[string]any
	require.NoError(t, json.Unmarshal(row.Meta, &meta))
	require.Equal(t, a.ID, meta["capability_id"])
}

func TestInferredReferenceDoesNotAddRolesToExplicitPermission(t *testing.T) {
	const pluginID = "com.example.plans"
	db := newMemoryDB(t)
	w := NewSyncWorker(SyncWorkerConfig{DB: db})
	role := iammodel.Role{TenantUUID: grantStatusTestTenant, Scope: string(coreiam.RoleScopeTenant), Code: "role_user", Name: "user"}
	require.NoError(t, db.Create(&role).Error)
	a := catalogCapability{ID: pluginID + ".read", Permissions: []pluginPermissionDeclaration{ownedAPIKeyDeclaration(pluginID, pluginID+".read")}}
	require.NoError(t, w.syncCapabilityPermissions(context.Background(), pluginMetadata{ID: pluginID}, a))
	b := catalogCapability{ID: pluginID + ".export", ToolScope: []string{"commerce.plans_service:read"}, DefaultRoleGrants: []string{"role_user"}}
	require.NoError(t, w.syncCapabilityPermissions(context.Background(), pluginMetadata{ID: pluginID}, b))
	var count int64
	require.NoError(t, db.Model(&iammodel.RolePermission{}).Where("role_id=?", role.ID).Count(&count).Error)
	require.Zero(t, count)
}

func TestCatalogConflictDoesNotPartiallyPublishCapabilities(t *testing.T) {
	const pluginID = "demo.plugin"
	db := newMemoryDB(t)
	root := buildSamplePlugin(t, true)
	a := catalogCapability{ID: pluginID + ".read", Permissions: []pluginPermissionDeclaration{ownedAPIKeyDeclaration(pluginID, pluginID+".read")}}
	b := catalogCapability{ID: pluginID + ".export", Permissions: []pluginPermissionDeclaration{ownedAPIKeyDeclaration(pluginID, pluginID+".export")}}
	raw, err := json.Marshal(map[string]any{"plugin": pluginMetadata{ID: pluginID, Version: "1.0.0"}, "capabilities": []catalogCapability{a, b}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "capabilities", "catalog.json"), raw, 0o600))
	err = NewSyncWorker(SyncWorkerConfig{DB: db}).ProcessArtifact(context.Background(), root)
	require.ErrorIs(t, err, iamrepo.ErrPluginAPIKeyPermissionConflict)
	for _, model := range []any{&capmodel.CapabilityRecord{}, &capmodel.CapabilitySyncJob{}, &iammodel.Permission{}} {
		var count int64
		require.NoError(t, db.Model(model).Count(&count).Error)
		require.Zero(t, count)
	}
}

func TestPluginCapabilityKeepsSeparateAdminAndServicePermissions(t *testing.T) {
	const pluginID = "com.example.plans"
	service := ownedAPIKeyDeclaration(pluginID, pluginID+".read")
	admin := service
	admin.PermissionCode, admin.Resource, admin.APIKey = "commerce.plans_admin:read", "plans_admin", nil
	admin.ProtocolBindings = []permissionProtocolBinding{{Channel: "rest", Method: "GET", Path: "/admin/plans", ActorContext: "admin_user", ResourceScope: "tenant"}}
	capability := catalogCapability{ID: pluginID + ".read", Permissions: []pluginPermissionDeclaration{service, admin}, Protocols: []capmodel.ProtocolBinding{
		{Channel: "rest", Method: "POST", Endpoint: "/plugin/plans/query", ActorContext: "service_actor", ResourceScope: "tenant"},
		{Channel: "rest", Method: "GET", Endpoint: "/admin/plans", ActorContext: "admin_user", ResourceScope: "tenant"},
	}}
	require.NoError(t, validatePluginPermissionDeclarations(pluginID, capability.ID, capability.Protocols, capability.Permissions))
	require.NoError(t, validateCatalogAPIKeyPermissionOwnership(pluginMetadata{ID: pluginID}, []catalogCapability{capability}))
	db := newMemoryDB(t)
	require.NoError(t, NewSyncWorker(SyncWorkerConfig{DB: db}).syncCapabilityPermissions(context.Background(), pluginMetadata{ID: pluginID}, capability))
	var rows []iammodel.Permission
	require.NoError(t, db.Order("resource").Find(&rows).Error)
	require.Len(t, rows, 2)
	require.Equal(t, "plans_admin", rows[0].Resource)
	require.False(t, rows[0].AllowAPIKey)
	require.Equal(t, "plans_service", rows[1].Resource)
	require.True(t, rows[1].AllowAPIKey)
}

func TestPluginPermissionConflictRollsBackWholeBatch(t *testing.T) {
	const pluginID = "com.example.plans"
	db := newMemoryDB(t)
	w := NewSyncWorker(SyncWorkerConfig{DB: db})
	a := catalogCapability{ID: pluginID + ".read", Permissions: []pluginPermissionDeclaration{ownedAPIKeyDeclaration(pluginID, pluginID+".read")}}
	require.NoError(t, w.syncCapabilityPermissions(context.Background(), pluginMetadata{ID: pluginID}, a))
	b := catalogCapability{ID: pluginID + ".export", Permissions: []pluginPermissionDeclaration{ownedAPIKeyDeclaration(pluginID, pluginID+".export")}}
	conflict, err := permissionRowFromPluginDeclaration(pluginMetadata{ID: pluginID}, b, b.Permissions[0])
	require.NoError(t, err)
	fresh := conflict
	fresh.Resource = "new_plans_service"
	accepted, err := w.permissionRepo.UpsertPluginCapabilityPermissions(context.Background(), []iammodel.Permission{fresh, conflict})
	require.ErrorIs(t, err, iamrepo.ErrPluginAPIKeyPermissionConflict)
	require.Empty(t, accepted)
	var count int64
	require.NoError(t, db.Model(&iammodel.Permission{}).Where("resource=?", fresh.Resource).Count(&count).Error)
	require.Zero(t, count)
	// 安装／启动的全量权限同步同样不能绕过 API Key 声明冲突保护。
	other := "com.example.other"
	conflict, err = permissionRowFromPluginDeclaration(pluginMetadata{ID: other}, catalogCapability{ID: other + ".read"}, ownedAPIKeyDeclaration(other, other+".read"))
	require.NoError(t, err)
	_, err = w.permissionRepo.Sync(context.Background(), "plugin:"+other, "1.0.0", []iammodel.Permission{conflict}, false)
	require.ErrorIs(t, err, iamrepo.ErrPluginAPIKeyPermissionConflict)
}
