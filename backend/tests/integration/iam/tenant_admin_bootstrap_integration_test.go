package iamintegration

import (
	"context"
	"testing"

	coreiam "github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	modeltenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/stretchr/testify/require"
)

func TestTenantAdminBootstrapIntegration(t *testing.T) {
	fx := setupIAMFixture(t)

	tenantUUID := "11111111-1111-4111-8111-111111111111"
	userID := uint64(1001)

	tenant := &modeltenant.Tenant{
		PowerUUIDModel: modeltenant.Tenant{}.PowerUUIDModel,
		Key:            "tenant-a",
		Name:           "Tenant A",
		Status:         1,
		Type:           modeltenant.TenantTypeEnterprise,
		Plan:           modeltenant.TenantPlanFree,
	}
	tenant.UUID = mustUUID(tenantUUID)
	require.NoError(t, fx.DB.Create(tenant).Error)

	user := &modeliam.User{
		Email:       "tenant-admin@example.com",
		Phone:       "13800000001",
		DisplayName: "Tenant Admin",
		Status:      modeliam.UserStatusActive,
	}
	user.ID = userID
	require.NoError(t, fx.DB.Create(user).Error)

	member := &modeliam.Member{
		TenantUUID:  tenantUUID,
		UserID:      userID,
		Username:    "tenant_admin",
		DisplayName: "Tenant Admin",
		Status:      modeliam.UserStatusActive,
	}
	require.NoError(t, fx.DB.Create(member).Error)

	role := &modeliam.Role{
		Scope:      string(coreiam.RoleScopeTenant),
		TenantUUID: tenantUUID,
		Code:       coreiam.CodeRoleAdmin,
		Name:       "Tenant Admin",
		Builtin:    true,
	}
	require.NoError(t, fx.DB.Create(role).Error)

	rb := &modeliam.RoleBinding{
		TenantUUID:  tenantUUID,
		RoleID:      role.ID,
		SubjectType: modeliam.SubMember,
		SubjectID:   member.ID,
	}
	require.NoError(t, fx.DB.Create(rb).Error)

	ctx := context.Background()
	ctx = reqctx.WithUserID(ctx, userID)
	ctx = reqctx.WithTenantUUID(ctx, tenantUUID)
	ctx = reqctx.WithMemberID(ctx, member.ID)

	resp, err := fx.Me.GetMeContext(ctx)
	require.NoError(t, err)
	require.False(t, resp.IsRoot)
	require.Equal(t, tenantUUID, resp.CurrentTenantUUID)
	require.NotNil(t, resp.CurrentMemberID)
	require.Equal(t, member.ID, *resp.CurrentMemberID)
	require.Len(t, resp.Members, 1)
	require.True(t, resp.Members[0].IsAdmin, "tenant admin should be resolved from role_binding + role_admin")
}
