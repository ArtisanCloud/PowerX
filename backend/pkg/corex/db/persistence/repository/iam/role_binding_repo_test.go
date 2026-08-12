package iam

import (
	"context"
	"fmt"
	"testing"

	modelbase "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRoleBindingRepositoryCreateIsIdempotentWithoutUniqueIndex(t *testing.T) {
	db := setupRoleBindingRepoTestDB(t)
	repo := NewRoleBindingRepository(db)
	ctx := context.Background()

	tenantUUID := uuid.NewString()
	roleUUID := uuid.NewString()
	memberUUID := uuid.NewString()

	first := &dbm.RoleBinding{
		TenantUUID:  tenantUUID,
		RoleUUID:    roleUUID,
		RoleID:      4,
		SubjectType: dbm.SubMember,
		SubjectUUID: memberUUID,
		SubjectID:   1,
	}
	require.NoError(t, repo.Create(ctx, first))
	require.NotZero(t, first.ID)

	second := &dbm.RoleBinding{
		TenantUUID:  tenantUUID,
		RoleUUID:    roleUUID,
		RoleID:      4,
		SubjectType: dbm.SubMember,
		SubjectUUID: memberUUID,
		SubjectID:   1,
	}
	require.NoError(t, repo.Create(ctx, second))
	require.Equal(t, first.ID, second.ID)

	var count int64
	require.NoError(t, db.Model(&dbm.RoleBinding{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestRoleBindingRepositoryAssignRolesByCodesIsIdempotentWithoutUniqueIndex(t *testing.T) {
	db := setupRoleBindingRepoTestDB(t)
	repo := NewRoleBindingRepository(db)
	ctx := context.Background()

	tenantUUID := uuid.NewString()
	role := dbm.Role{
		Scope:      "tenant",
		TenantUUID: tenantUUID,
		Code:       "role_admin",
		Name:       "Admin",
	}
	require.NoError(t, db.Create(&role).Error)
	member := dbm.Member{
		TenantUUID:  tenantUUID,
		UserUUID:    uuid.NewString(),
		UserID:      7,
		Username:    "member",
		DisplayName: "Member",
		Status:      1,
	}
	require.NoError(t, db.Create(&member).Error)

	require.NoError(t, repo.AssignRolesByCodes(ctx, tenantUUID, member.ID, "role_admin"))
	require.NoError(t, repo.AssignRolesByCodes(ctx, tenantUUID, member.ID, "role_admin"))

	var count int64
	require.NoError(t, db.Model(&dbm.RoleBinding{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func setupRoleBindingRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	prevSchema := modelbase.PowerXSchema
	modelbase.PowerXSchema = "main"
	t.Cleanup(func() { modelbase.PowerXSchema = prevSchema })

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared&_loc=UTC", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&dbm.Role{},
		&dbm.Member{},
		&dbm.RoleBinding{},
	))
	return db
}
