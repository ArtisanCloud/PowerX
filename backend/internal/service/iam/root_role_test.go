package iam

import (
	"context"
	"testing"

	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	coreiam "github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/rootrole"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRootRoleIsCoreManagedAcrossRoleAndRBACWrites(t *testing.T) {
	db := setupRBACServiceTestDB(t)
	ctx := context.Background()
	_, err := rootrole.Ensure(db)
	require.NoError(t, err)
	var root im.Role
	require.NoError(t, db.Where("scope=? AND code=?", "system", coreiam.CodeSystemRoot).Take(&root).Error)
	roles, rbac := NewRoleService(db), NewRBACService(db)
	actor, scope := ActorContext{IsRoot: true}, RoleContext{IsRoot: true}
	for _, role := range []im.Role{{Scope: "system", Code: "root"}, {Scope: "tenant", TenantUUID: "tenant", Code: " ROOT "}} {
		_, err := roles.Create(ctx, scope, &role)
		require.ErrorIs(t, err, rootrole.ErrManaged)
	}
	// Even a legacy root row missing builtin protection stays immutable to the API.
	require.NoError(t, db.Model(&root).Update("builtin", false).Error)
	paddedRoot := root
	paddedRoot.TenantUUID = "                                    "
	require.True(t, rootrole.IsManaged(paddedRoot), "PostgreSQL char(36) pads the empty system tenant")
	for _, write := range []func() error{
		func() error { return roles.Update(ctx, scope, root.ID, &im.Role{Name: "renamed"}) },
		func() error { return roles.Delete(ctx, scope, root.ID) },
		func() error { return rbac.GrantPermsByIDs(ctx, actor, root.ID, []uint64{999}) },
		func() error {
			return rbac.GrantPermsByTriples(ctx, actor, root.ID, []PermTriple{{Module: "fake", Resource: "forbidden", Action: "read"}})
		},
		func() error { return rbac.RevokePermissionsFromRole(ctx, actor, root.ID, []uint64{999}) },
		func() error { _, err := rbac.SetPermissionIDs(ctx, actor, root.ID, nil); return err },
	} {
		require.ErrorIs(t, write(), rootrole.ErrManaged)
	}
	var permissions int64
	require.NoError(t, db.Model(&im.Permission{}).Count(&permissions).Error)
	require.Zero(t, permissions, "a rejected root grant must not register arbitrary permissions")
	require.Error(t, rbac.BindRoleToMember(ctx, actor, "tenant", root.ID, 1))
	_, err = rootrole.Ensure(db)
	require.NoError(t, err)
	var after im.Role
	require.NoError(t, db.First(&after, root.ID).Error)
	require.Equal(t, root.UUID, after.UUID)
	require.Equal(t, root.Name, after.Name)
	require.True(t, after.Builtin)
	// Tenant admin retains its independent editable role and receives no automatic grants.
	admin, err := roles.Create(ctx, scope, &im.Role{Scope: "tenant", TenantUUID: "tenant", Code: "role_admin", Name: "Admin"})
	require.NoError(t, err)
	require.NoError(t, roles.Update(ctx, scope, admin.ID, &im.Role{Name: "Tenant Admin"}))
	require.NoError(t, rbac.GrantPermsByTriples(ctx, actor, admin.ID, []PermTriple{{Module: "business", Resource: "orders", Action: "read"}}))
}

func TestPermissionSyncMaintainsRootCatalogSynchronouslyAndRollsBackOnFailure(t *testing.T) {
	db := setupRBACServiceTestDB(t)
	svc := NewPermissionService(db)
	ctx := context.Background()
	row := im.Permission{Module: "operations", Resource: "cost", Action: "read"}
	_, err := svc.SyncPermissions(ctx, "plugin:com.example.business", "1", []im.Permission{row}, true)
	require.NoError(t, err)
	var count int64
	require.NoError(t, db.Model(&im.Role{}).Count(&count).Error)
	require.Zero(t, count, "dry run must not create root")
	_, err = svc.SyncPermissions(ctx, "plugin:com.example.business", "1", []im.Permission{row}, false)
	require.NoError(t, err)
	var root im.Role
	require.NoError(t, db.Where("scope=? AND code=?", "system", coreiam.CodeSystemRoot).Take(&root).Error)
	require.NoError(t, db.Model(&im.RolePermission{}).Where("role_uuid=?", root.UUID).Count(&count).Error)
	require.EqualValues(t, 1, count, "the grant must exist before SyncPermissions returns")
	_, err = svc.SyncPermissions(ctx, "plugin:com.example.business", "1", []im.Permission{row}, false)
	require.NoError(t, err)
	require.NoError(t, db.Model(&im.RolePermission{}).Where("role_uuid=?", root.UUID).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:reject_root_grant", func(tx *gorm.DB) {
		if tx.Statement.Table == (&im.RolePermission{}).GetTableName(false) {
			tx.AddError(rootrole.ErrManaged)
		}
	}))
	defer db.Callback().Create().Remove("test:reject_root_grant")
	_, err = svc.SyncPermissions(ctx, "plugin:com.example.more", "1", []im.Permission{{Module: "operations", Resource: "more", Action: "read"}}, false)
	require.ErrorIs(t, err, rootrole.ErrManaged)
	require.NoError(t, db.Model(&im.Permission{}).Where("source=?", "plugin:com.example.more").Count(&count).Error)
	require.Zero(t, count, "registration must roll back when its managed catalog grant fails")
}
