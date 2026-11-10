package seed

import (
	"context"
	"os"
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	infraiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func roleIdentityDB(t *testing.T) *gorm.DB {
	t.Helper()
	previous := coremodel.PowerXSchema
	dsn := os.Getenv("POWERX_ROLE_IDENTITY_TEST_DSN")
	coremodel.PowerXSchema = "main"
	if dsn != "" {
		coremodel.PowerXSchema = "role_identity_" + uuid.NewString()[:8]
	}
	schema := coremodel.PowerXSchema
	t.Cleanup(func() { coremodel.PowerXSchema = previous })
	var dialect gorm.Dialector = sqlite.Open("file:" + uuid.NewString() + "?mode=memory&cache=shared")
	if dsn != "" {
		dialect = postgres.Open(dsn)
	}
	db, err := gorm.Open(dialect, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	if dsn != "" {
		require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
	}
	t.Cleanup(func() {
		if dsn != "" {
			require.NoError(t, db.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
		}
		require.NoError(t, pool.Close())
	})
	require.NoError(t, db.AutoMigrate(&dbm.Role{}, &dbm.Permission{}, &dbm.RolePermission{}))
	return db
}

func TestRepeatedBuiltInRoleSeedPreservesIdentityAndMultipleRolePermissions(t *testing.T) {
	db := roleIdentityDB(t)
	const tenant = "11111111-1111-4111-8111-111111111111"
	permissions := []dbm.Permission{
		{Module: "production", Resource: "sample_track", Action: "read", Effect: "allow", Status: dbm.PermissionStatusActive, Meta: []byte(`{"module":"production"}`)},
		{Module: "production", Resource: "sample_track", Action: "write", Effect: "allow", Status: dbm.PermissionStatusActive, Meta: []byte(`{"module":"production"}`)},
	}
	require.NoError(t, db.Create(&permissions).Error)
	custom := dbm.Role{Scope: "tenant", TenantUUID: tenant, Code: "custom_viewer", Name: "Existing Viewer"}
	require.NoError(t, db.Create(&custom).Error)
	repo := infraiam.NewRoleRepository(db)
	permissionRepo := infraiam.NewRolePermissionRepository(db)
	require.NoError(t, permissionRepo.BindPermissions(context.Background(), custom.ID, permissions[0].ID))
	require.NoError(t, db.First(&custom, custom.ID).Error)
	require.NoError(t, SeedBuiltInRolesAndGrants(db, tenant))
	var before []dbm.Role
	var beforePermissions []dbm.RolePermission
	require.NoError(t, db.Order("id").Find(&before).Error)
	require.NoError(t, db.Order("role_id, permission_id").Find(&beforePermissions).Error)
	for repeat := 0; repeat < 2; repeat++ {
		require.NoError(t, SeedBuiltInRolesAndGrants(db, tenant))
		var after []dbm.Role
		var afterPermissions []dbm.RolePermission
		require.NoError(t, db.Order("id").Find(&after).Error)
		require.Len(t, after, len(before))
		for i := range before {
			require.Equal(t, before[i].ID, after[i].ID, string(before[i].Code))
			require.Equal(t, before[i].UUID, after[i].UUID, string(before[i].Code))
			require.Equal(t, before[i].CreatedAt, after[i].CreatedAt, string(before[i].Code))
		}
		require.NoError(t, db.Order("role_id, permission_id").Find(&afterPermissions).Error)
		require.Equal(t, beforePermissions, afterPermissions)
	}
	// 同一读取权限可以同时属于多个角色；不把业务角色替换成 Root。
	for _, roleKey := range []struct{ scope, tenant, code string }{
		{"system", "", "root"}, {"system", "", "system_monitor"},
		{"tenant", tenant, "role_admin"}, {"tenant", tenant, "role_readonly"}, {"tenant", tenant, "custom_viewer"},
	} {
		role, err := repo.FindByCode(context.Background(), roleKey.scope, &roleKey.tenant, roleKey.code)
		require.NoError(t, err)
		var count int64
		require.NoError(t, db.Model(&dbm.RolePermission{}).Where("role_id=? AND role_uuid=? AND permission_uuid=?", role.ID, role.UUID, permissions[0].PermissionUUID).Count(&count).Error)
		require.EqualValues(t, 1, count, roleKey.code)
	}
	var currentCustom dbm.Role
	require.NoError(t, db.First(&currentCustom, custom.ID).Error)
	require.Equal(t, custom, currentCustom)
}

func TestSystemRoleMetadataUpdateReturnsOriginalIdentity(t *testing.T) {
	db := roleIdentityDB(t)
	repo := infraiam.NewRoleRepository(db)
	first, err := repo.UpsertPreservingIdentity(context.Background(), &dbm.Role{Scope: "system", Code: "root", Name: "Old Name", Builtin: true})
	require.NoError(t, err)
	before := *first
	proposed := &dbm.Role{Scope: "system", Code: "root", Name: "Super Admin", Builtin: true}
	stored, err := repo.UpsertPreservingIdentity(context.Background(), proposed)
	require.NoError(t, err)
	require.Same(t, proposed, stored)
	require.Equal(t, before.UUID, stored.UUID)
	require.Equal(t, before.ID, stored.ID)
	require.Equal(t, before.CreatedAt, stored.CreatedAt)
	require.Equal(t, "Super Admin", stored.Name)
}

func TestRepeatedSeedDoesNotRewriteLegacyDanglingRoleLinks(t *testing.T) {
	db := roleIdentityDB(t)
	const tenant = "11111111-1111-4111-8111-111111111111"
	permission := dbm.Permission{Module: "production", Resource: "sample_track", Action: "read", Effect: "allow", Status: dbm.PermissionStatusActive, Meta: []byte(`{"module":"production"}`)}
	require.NoError(t, db.Create(&permission).Error)
	require.NoError(t, SeedBuiltInRolesAndGrants(db, tenant))
	var root dbm.Role
	require.NoError(t, db.Where("scope=? AND code=?", "system", "root").Take(&root).Error)
	stale := uuid.New()
	require.NoError(t, db.Model(&dbm.RolePermission{}).Where("role_id=? AND permission_id=?", root.ID, permission.ID).Update("role_uuid", stale).Error)
	require.NoError(t, SeedBuiltInRolesAndGrants(db, tenant))
	var after dbm.Role
	var link dbm.RolePermission
	require.NoError(t, db.First(&after, root.ID).Error)
	require.NoError(t, db.Where("role_id=? AND permission_id=?", root.ID, permission.ID).Take(&link).Error)
	require.Equal(t, root.UUID, after.UUID)
	require.Equal(t, stale, link.RoleUUID)
}
