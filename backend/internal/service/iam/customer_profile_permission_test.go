package iam

import (
	"context"
	"testing"
	"time"

	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCustomerProfilePermissionCheckUsesLiveUUIDGrant(t *testing.T) {
	db := setupRBACServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&im.RoleBinding{}, &im.MemberAssignment{}))
	tenant, member := uuid.NewString(), uuid.NewString()
	ctx := reqctx.WithMemberUUID(context.Background(), member)
	role := im.Role{TenantUUID: tenant, Scope: "tenant", Code: "role_user", Name: "User"}
	require.NoError(t, db.Create(&role).Error)
	binding := im.RoleBinding{TenantUUID: tenant, RoleID: role.ID, RoleUUID: role.UUID.String(), SubjectType: im.SubMember, SubjectID: 7, SubjectUUID: member}
	require.NoError(t, db.Create(&binding).Error)
	permission := im.Permission{Module: "customer", Resource: "admin_customers_accounts_customer_uuid", Action: "update", Source: "platform_capability", Effect: "allow", Status: "active"}
	require.NoError(t, db.Create(&permission).Error)
	grant := im.RolePermission{RoleID: role.ID, RoleUUID: role.UUID, PermissionID: permission.ID, PermissionUUID: permission.PermissionUUID}
	require.NoError(t, db.Create(&grant).Error)
	check := func(want bool) {
		ok, err := NewRBACService(db).Enforce(ctx, ActorContext{TenantUUID: tenant}, tenant, 7, permission.Module, permission.Resource, permission.Action)
		require.NoError(t, err)
		require.Equal(t, want, ok)
	}
	check(true)
	for _, fields := range []map[string]any{{"effect": "deny"}, {"effect": "allow", "status": "deprecated"}, {"status": "active", "source": "platform_capability_generated"}} {
		require.NoError(t, db.Model(&permission).Updates(fields).Error)
		check(false)
	}
	require.NoError(t, db.Model(&permission).Update("source", "platform_capability").Error)
	end := time.Now().Add(-time.Minute).UnixMilli()
	require.NoError(t, db.Model(&binding).Update("end_at", end).Error)
	check(false)
	require.NoError(t, db.Model(&binding).Update("end_at", nil).Error)
	check(true)
	require.NoError(t, db.Delete(&grant).Error)
	check(false)
}
