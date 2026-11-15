package integration_gateway

import (
	"context"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	iam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	tenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestIAM434FormalRoleManagementCatalogUpgradePreservesLegacyGrants(t *testing.T) {
	db := newCatalogUpgradeDB(t)
	require.NoError(t, db.AutoMigrate(&iam.Permission{}, &iam.Role{}, &iam.RolePermission{}))
	row := &tenant.Tenant{Key: "iam434-fixture", Name: "IAM434 Fixture", Status: 1}
	row.UUID = uuid.New()
	require.NoError(t, db.Create(row).Error)
	legacy := iam.Permission{Module: "iam", Resource: "role", Action: "write", Source: "plugin:com.powerx.plugins.ai-craft", Status: "active", Effect: "allow"}
	require.NoError(t, db.Create(&legacy).Error)
	role := iam.Role{Scope: "tenant", TenantUUID: row.UUID.String(), Code: "role_admin", Name: "Tenant Admin"}
	require.NoError(t, db.Create(&role).Error)
	require.NoError(t, db.Create(&iam.RolePermission{RoleID: role.ID, RoleUUID: role.UUID, PermissionID: legacy.ID, PermissionUUID: legacy.PermissionUUID}).Error)
	ids := []string{"com.corex.iam.role_management.admin_read", "com.corex.iam.role_management.admin_write", "com.corex.iam.role_management.admin_delete", "com.corex.iam.role_management.admin_bind"}
	seeder := catalogSeeder(db)
	seeder.capabilityIDs = ids
	ctx := context.Background()
	for n := 0; n < 2; n++ {
		require.NoError(t, apikeypermissions.EnsureUpgradePermissionsForCapabilities(ctx, db, ids))
		require.NoError(t, seeder.Ensure(ctx))
	}
	registry := repo.NewCapabilityRegistryRepository(db)
	for _, id := range ids {
		registration, err := registry.GetLatest(ctx, nil, id, row.UUID.String())
		require.NoError(t, err)
		require.EqualValues(t, 1, registration.Version)
		require.NotEmpty(t, registration.Adapters)
		for _, adapter := range registration.Adapters {
			require.Equal(t, "http", adapter.TransportType)
		}
	}
	var current iam.Permission
	require.NoError(t, db.First(&current, legacy.ID).Error)
	require.Equal(t, legacy.Source, current.Source)
	require.Equal(t, legacy.PermissionUUID, current.PermissionUUID)
	var count int64
	require.NoError(t, db.Model(&iam.RolePermission{}).Count(&count).Error)
	require.EqualValues(t, 1, count, "publishing a catalog must not give roles new management grants")
	var permissions []iam.Permission
	require.NoError(t, db.Where("resource = ?", "role_management").Find(&permissions).Error)
	require.Len(t, permissions, 4)
	for _, permission := range permissions {
		require.Equal(t, "platform_capability", permission.Source)
		require.False(t, permission.AllowAPIKey)
	}
}
