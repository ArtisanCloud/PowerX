package customer

import (
	"context"
	"testing"
	"time"

	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCustomerAdminCreationRequiresLiveAPIPermission(t *testing.T) {
	db, tenant, _ := newContactServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&im.Permission{}, &im.Role{}, &im.RolePermission{}, &im.RoleBinding{}, &im.MemberAssignment{}))
	memberUUID := uuid.NewString()
	ctx := reqctx.WithTenantUUID(context.Background(), tenant)
	ctx = reqctx.WithUserID(ctx, 42)
	ctx = reqctx.WithMemberID(ctx, 7)
	ctx = reqctx.WithMemberUUID(ctx, memberUUID)
	role := im.Role{TenantUUID: tenant, Scope: "tenant", Code: "role_user", Name: "User"}
	require.NoError(t, db.Create(&role).Error)
	binding := im.RoleBinding{TenantUUID: tenant, RoleUUID: role.UUID.String(), RoleID: role.ID, SubjectType: im.SubMember, SubjectUUID: memberUUID, SubjectID: 7}
	require.NoError(t, db.Create(&binding).Error)
	menu := im.Permission{Module: "menu", Resource: "customer", Action: "view", Source: "core"}
	require.NoError(t, db.Create(&menu).Error)
	require.NoError(t, db.Create(&im.RolePermission{RoleID: role.ID, RoleUUID: role.UUID, PermissionID: menu.ID, PermissionUUID: menu.PermissionUUID}).Error)
	input := CreateAccountInput{TenantUUID: tenant, Type: "person", DisplayName: "Ada", PrimaryEmail: "ada@example.test"}
	s := NewAccountService(db)
	_, err := s.CreateFromAdmin(ctx, input)
	require.Equal(t, 403, dto.StatusCode(err), "menu grant must not allow creation")
	permission := im.Permission{Module: "customer", Resource: "admin_customers_accounts", Action: "create", Source: "platform_capability"}
	require.NoError(t, db.Create(&permission).Error)
	grant := im.RolePermission{RoleID: role.ID, RoleUUID: role.UUID, PermissionID: permission.ID, PermissionUUID: permission.PermissionUUID}
	require.NoError(t, db.Create(&grant).Error)
	created, err := s.CreateFromAdmin(ctx, input)
	require.NoError(t, err)
	require.NotEmpty(t, created.PrimaryContactUUID)
	_, err = NewAccountImportService(db).Preview(ctx, "customers.csv", "display_name,primary_email\nAnother,another@example.test\n")
	require.Equal(t, 403, dto.StatusCode(err), "create grant must not allow import")
	for _, change := range []map[string]any{{"effect": "deny"}, {"effect": "allow", "status": "deprecated"}} {
		require.NoError(t, db.Model(&permission).Updates(change).Error)
		_, err := s.CreateFromAdmin(ctx, input)
		require.Equal(t, 403, dto.StatusCode(err))
	}
	require.NoError(t, db.Model(&permission).Updates(map[string]any{"effect": "allow", "status": "active"}).Error)
	end := time.Now().Add(-time.Minute).UnixMilli()
	require.NoError(t, db.Model(&binding).Update("end_at", end).Error)
	_, err = s.CreateFromAdmin(ctx, input)
	require.Equal(t, 403, dto.StatusCode(err))
	require.NoError(t, db.Model(&binding).Update("end_at", nil).Error)
	require.NoError(t, db.Delete(&permission).Error)
	_, err = s.CreateFromAdmin(ctx, input)
	require.Equal(t, 403, dto.StatusCode(err))
}
