package iam

import (
	"context"
	"fmt"
	"testing"

	modelbase "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMemberHasPermissionViaBindingUsesEffectiveAssignments(t *testing.T) {
	db := setupPermissionRepoTestDB(t)
	ctx := context.Background()
	repo := NewPermissionRepository(db)

	const tenantUUID = "tenant-uuid"
	const memberID uint64 = 42

	perm := createPermission(t, db, "corex.customer", "accounts", "list")
	require.NoError(t, db.Create(&dbm.RolePermission{RoleID: 100, PermissionID: perm.ID}).Error)
	require.NoError(t, db.Create(&dbm.RoleBinding{
		TenantUUID:  tenantUUID,
		RoleID:      100,
		SubjectType: dbm.SubTeam,
		SubjectID:   7,
	}).Error)
	require.NoError(t, db.Create(&dbm.MemberAssignment{
		TenantUUID: tenantUUID,
		MemberID:   memberID,
		DimType:    dbm.DimTeam,
		DimID:      7,
	}).Error)

	ok, err := repo.MemberHasPermissionViaBindingWithModule(ctx, tenantUUID, memberID, "corex.customer", "accounts", "list")
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = repo.MemberHasPermissionViaBindingWithModule(ctx, tenantUUID, memberID, "corex.customer", "accounts", "delete")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestMemberHasPermissionViaBindingMapsOrgAssignmentToOrgUnitSubject(t *testing.T) {
	db := setupPermissionRepoTestDB(t)
	ctx := context.Background()
	repo := NewPermissionRepository(db)

	const tenantUUID = "tenant-uuid"
	const memberID uint64 = 42

	perm := createPermission(t, db, "corex.iam", "members", "read")
	require.NoError(t, db.Create(&dbm.RolePermission{RoleID: 200, PermissionID: perm.ID}).Error)
	require.NoError(t, db.Create(&dbm.RoleBinding{
		TenantUUID:  tenantUUID,
		RoleID:      200,
		SubjectType: dbm.SubOrg,
		SubjectID:   88,
	}).Error)
	require.NoError(t, db.Create(&dbm.MemberAssignment{
		TenantUUID: tenantUUID,
		MemberID:   memberID,
		DimType:    dbm.DimOrg,
		DimID:      88,
	}).Error)

	ok, err := repo.MemberHasPermissionViaBindingWithModule(ctx, tenantUUID, memberID, "corex.iam", "members", "read")
	require.NoError(t, err)
	require.True(t, ok)
}

func TestMemberHasPermissionViaBindingStillSupportsDirectMemberBinding(t *testing.T) {
	db := setupPermissionRepoTestDB(t)
	ctx := context.Background()
	repo := NewPermissionRepository(db)

	const tenantUUID = "tenant-uuid"
	const memberID uint64 = 42

	perm := createPermission(t, db, "corex.agent", "agents", "read")
	require.NoError(t, db.Create(&dbm.RolePermission{RoleID: 300, PermissionID: perm.ID}).Error)
	require.NoError(t, db.Create(&dbm.RoleBinding{
		TenantUUID:  tenantUUID,
		RoleID:      300,
		SubjectType: dbm.SubMember,
		SubjectID:   memberID,
	}).Error)

	ok, err := repo.MemberHasPermissionViaBindingWithModule(ctx, tenantUUID, memberID, "corex.agent", "agents", "read")
	require.NoError(t, err)
	require.True(t, ok)
}

func setupPermissionRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	prevSchema := modelbase.PowerXSchema
	modelbase.PowerXSchema = "main"
	t.Cleanup(func() { modelbase.PowerXSchema = prevSchema })

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared&_loc=UTC", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&dbm.Permission{},
		&dbm.RolePermission{},
		&dbm.RoleBinding{},
		&dbm.MemberAssignment{},
	))
	return db
}

func createPermission(t *testing.T, db *gorm.DB, module, resource, action string) dbm.Permission {
	t.Helper()
	perm := dbm.Permission{
		Module:   module,
		Resource: resource,
		Action:   action,
		Effect:   "allow",
		Status:   dbm.PermissionStatusActive,
	}
	require.NoError(t, db.Create(&perm).Error)
	return perm
}
