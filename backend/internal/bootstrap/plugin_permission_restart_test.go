package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	pmimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// 使用真实加载器、注册表、权限同步和路由解析；所有文件和表均为隔离资源。
func TestPluginPermissionSyncRestartRepairsDeprecatedRoutesAndPreservesGrants(t *testing.T) {
	ctx := context.Background()
	db := newPluginAuthzTestDB(t)
	require.NoError(t, db.AutoMigrate(&im.Role{}, &im.RolePermission{}))
	const pluginID = "com.example.plugins.restart"
	common := restartRoutePermission("records_api", "read", "GET", "/admin/records")
	retired := restartRoutePermission("retired_api", "read", "GET", "/admin/retired")
	debugRead := restartRoutePermission("debug_api", "read", "GET", "/admin/debug")
	debugRun := restartRoutePermission("debug_api", "run", "POST", "/admin/debug")
	manifests := []plugin_mgr.Manifest{
		{ID: pluginID, Name: "Restart regression", Version: "0.1.8", Permissions: []plugin_mgr.PermissionSpec{common, retired}},
		{ID: pluginID, Name: "Restart regression", Version: "0.1.10", Permissions: []plugin_mgr.PermissionSpec{common}},
		{ID: pluginID, Name: "Restart regression", Version: "0.1.11", Permissions: []plugin_mgr.PermissionSpec{common, debugRead, debugRun}},
	}
	root := t.TempDir()
	registryPath := filepath.Join(root, "registry.json")
	installedRoot := filepath.Join(root, "installed")
	registry := pmimpl.NewJSONRegistry(registryPath)
	for i := range manifests {
		manifest := &manifests[i]
		manifest.Runtime.Kind = plugin_mgr.RuntimeKindStatic
		pluginRoot := filepath.Join(installedRoot, pluginID, manifest.Version)
		require.NoError(t, os.MkdirAll(pluginRoot, 0o755))
		raw, err := yaml.Marshal(manifest)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(pluginRoot, "plugin.yaml"), raw, 0o644))
		desc, err := pmimpl.NewFSLoader().LoadDescriptor(ctx, pluginRoot)
		require.NoError(t, err)
		require.NoError(t, registry.Put(ctx, desc, plugin_mgr.StateInstalled))
	}
	require.NoError(t, registry.UpdateState(ctx, pluginID, "0.1.11", plugin_mgr.StateEnabled))
	require.NoError(t, registry.Save(ctx))
	require.NoError(t, syncPluginManifestPermissions(ctx, db, manifests[0]))
	require.NoError(t, syncPluginManifestPermissions(ctx, db, manifests[2]))

	var original im.Permission
	require.NoError(t, db.Where("resource=? AND action=?", "debug_api", "read").Take(&original).Error)
	role := im.Role{Scope: "tenant", TenantUUID: uuid.NewString(), Code: "role_user", Name: "Existing business role"}
	require.NoError(t, db.Create(&role).Error)
	grant := im.RolePermission{RoleID: role.ID, RoleUUID: role.UUID, PermissionID: original.ID, PermissionUUID: original.PermissionUUID}
	require.NoError(t, db.Create(&grant).Error)
	// 复现旧启动逻辑的落库结果：最后同步旧版，新接口废弃，已移除接口复活。
	require.NoError(t, syncPluginManifestPermissions(ctx, db, manifests[0]))
	var broken im.Permission
	require.NoError(t, db.First(&broken, original.ID).Error)
	require.Equal(t, im.PermissionStatusDeprecated, broken.Status)
	before, err := (pluginIAMAuthorizer{db: db}).RoutePermission(ctx, pluginID, "GET", "/admin/debug")
	require.NoError(t, err)
	require.Nil(t, before)
	var grantsBefore []im.RolePermission
	require.NoError(t, db.Order("role_id, permission_id").Find(&grantsBefore).Error)

	for restart := 0; restart < 3; restart++ {
		var synced []string
		var syncErrors []error
		registry = pmimpl.NewJSONRegistry(registryPath)
		manager := pmimpl.New(pmimpl.Options{
			Enabled: true, InstalledRoot: installedRoot, Registry: registry, Loader: pmimpl.NewFSLoader(),
			PostInstallManifest: func(ctx context.Context, manifest plugin_mgr.Manifest) error {
				synced = append(synced, manifest.Version)
				err := syncPluginManifestPermissions(ctx, db, manifest)
				if err != nil {
					syncErrors = append(syncErrors, err)
				}
				return err
			},
		})
		require.NoError(t, manager.Bootstrap(ctx))
		require.Empty(t, syncErrors)
		require.Equal(t, []string{"0.1.11"}, synced)
		var after im.Permission
		require.NoError(t, db.First(&after, original.ID).Error)
		require.Equal(t, im.PermissionStatusActive, after.Status)
		require.Equal(t, original.PermissionUUID, after.PermissionUUID)
		require.Nil(t, after.DeprecatedAt)
		require.False(t, after.AllowAPIKey, "恢复登记不能附加服务 Key 权限")
		var grantsAfter []im.RolePermission
		require.NoError(t, db.Order("role_id, permission_id").Find(&grantsAfter).Error)
		require.Equal(t, grantsBefore, grantsAfter, "保留 Root 和现有业务角色授权关系")
		for _, method := range []string{"GET", "POST"} {
			permission, err := (pluginIAMAuthorizer{db: db}).RoutePermission(ctx, pluginID, method, "/admin/debug")
			require.NoError(t, err)
			require.NotNil(t, permission)
		}
		for _, target := range []struct{ plugin, path string }{
			{pluginID, "/admin/retired"}, {"com.example.plugins.other", "/admin/debug"},
		} {
			permission, err := (pluginIAMAuthorizer{db: db}).RoutePermission(ctx, target.plugin, "GET", target.path)
			require.NoError(t, err)
			require.Nil(t, permission, "不恢复已移除接口，也不跨插件匹配")
		}
	}
}

func restartRoutePermission(resource, action, method, path string) plugin_mgr.PermissionSpec {
	return plugin_mgr.PermissionSpec{
		Type: "api", PermissionCode: "restart." + resource + ":" + action,
		Independent: true, RiskLevel: "low", DataScope: "tenant",
		ProtocolBindings: []plugin_mgr.PermissionProtocolBinding{{
			Channel: "rest", Method: method, Path: path, ActorContext: "admin_user", ResourceScope: "tenant",
		}},
	}
}
