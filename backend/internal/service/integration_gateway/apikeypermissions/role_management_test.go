package apikeypermissions

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRoleManagementCatalogHasIndependentUserOnlyPermissionNamespace(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "config", "platform_capabilities", "iam_role_management.yaml"))
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "roles.yaml"), raw, 0600))
	t.Setenv(platformCapabilitiesDirEnv, dir)
	rows, err := loadPlatformCapabilityPermissions()
	require.NoError(t, err)
	actions := map[string]bool{}
	for _, row := range rows {
		require.Equal(t, "iam", row.Module)
		require.Equal(t, "platform_capability", row.Source)
		require.False(t, row.AllowAPIKey)
		require.NotEqual(t, "role", row.Resource, "do not reclaim or reuse a plugin-owned legacy permission")
		if row.Resource == "role_management" {
			actions[row.Action] = true
		}
	}
	require.Equal(t, map[string]bool{"read": true, "write": true, "delete": true, "bind": true}, actions)

	// The exact formal publication path preserves existing role grants and the
	// historical plugin permission UUID/owner; it must not silently grant admins.
	previous := core.PowerXSchema
	core.PowerXSchema = "main"
	t.Cleanup(func() { core.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	require.NoError(t, db.AutoMigrate(&m.Permission{}, &m.Role{}, &m.RolePermission{}))
	legacy := m.Permission{Module: "iam", Resource: "role", Action: "write", Source: "plugin:com.powerx.plugins.ai-craft", Status: "active", Effect: "allow"}
	require.NoError(t, db.Create(&legacy).Error)
	platformPermissionOnce = sync.Once{}
	platformPermissionRows, platformPermissionErr = nil, nil
	t.Cleanup(func() {
		platformPermissionOnce = sync.Once{}
		platformPermissionRows, platformPermissionErr = nil, nil
	})
	ids := []string{"com.corex.iam.role_management.admin_read", "com.corex.iam.role_management.admin_write", "com.corex.iam.role_management.admin_delete", "com.corex.iam.role_management.admin_bind"}
	require.NoError(t, EnsureUpgradePermissionsForCapabilities(context.Background(), db, ids))
	var restored m.Permission
	require.NoError(t, db.First(&restored, legacy.ID).Error)
	require.Equal(t, legacy.PermissionUUID, restored.PermissionUUID)
	require.Equal(t, legacy.Source, restored.Source)
	var count int64
	require.NoError(t, db.Model(&m.Permission{}).Where("resource = ? AND source = ?", "role_management", "platform_capability").Count(&count).Error)
	require.EqualValues(t, 4, count)
	require.NoError(t, db.Model(&m.RolePermission{}).Count(&count).Error)
	require.Zero(t, count)
}
