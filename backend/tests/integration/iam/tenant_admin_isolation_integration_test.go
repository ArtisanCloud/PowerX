package iamintegration

import (
	"context"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	modeltenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/stretchr/testify/require"
)

func TestTenantAdminIsolationIntegration(t *testing.T) {
	fx := setupIAMFixture(t)

	tenantA := &modeltenant.Tenant{
		Key:    "tenant-a",
		Name:   "Tenant A",
		Status: 1,
		Type:   modeltenant.TenantTypeEnterprise,
		Plan:   modeltenant.TenantPlanFree,
		Domain: "tenant-a.local",
	}
	tenantA.UUID = mustUUID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	require.NoError(t, fx.DB.Create(tenantA).Error)

	tenantB := &modeltenant.Tenant{
		Key:    "tenant-b",
		Name:   "Tenant B",
		Status: 1,
		Type:   modeltenant.TenantTypeEnterprise,
		Plan:   modeltenant.TenantPlanFree,
		Domain: "tenant-b.local",
	}
	tenantB.UUID = mustUUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
	require.NoError(t, fx.DB.Create(tenantB).Error)

	memberUser := &modeliam.User{
		Email:       "member@example.com",
		Phone:       "13800000002",
		DisplayName: "Member",
		Status:      modeliam.UserStatusActive,
		IsRoot:      false,
	}
	require.NoError(t, fx.DB.Create(memberUser).Error)

	member := &modeliam.Member{
		TenantUUID:  tenantA.UUID.String(),
		UserID:      memberUser.ID,
		Username:    "member_a",
		DisplayName: "Member A",
		Status:      modeliam.UserStatusActive,
	}
	require.NoError(t, fx.DB.Create(member).Error)

	t.Run("non-root stale tenant should fallback to membership tenant", func(t *testing.T) {
		ctx := context.Background()
		ctx = reqctx.WithUserID(ctx, memberUser.ID)
		ctx = reqctx.WithTenantUUID(ctx, tenantB.UUID.String()) // stale / non-membership tenant

		resp, err := fx.Me.GetMeContext(ctx)
		require.NoError(t, err)
		require.False(t, resp.IsRoot)
		require.Equal(t, tenantA.UUID.String(), resp.CurrentTenantUUID)
		require.NotNil(t, resp.CurrentMemberID)
		require.Equal(t, member.ID, *resp.CurrentMemberID)
	})

	t.Run("root can keep non-membership tenant when tenant exists", func(t *testing.T) {
		rootUser := &modeliam.User{
			Email:       "root@example.com",
			Phone:       "13800000003",
			DisplayName: "Root",
			Status:      modeliam.UserStatusActive,
			IsRoot:      true,
		}
		require.NoError(t, fx.DB.Create(rootUser).Error)

		rootMember := &modeliam.Member{
			TenantUUID:  tenantA.UUID.String(),
			UserID:      rootUser.ID,
			Username:    "root_a",
			DisplayName: "Root A",
			Status:      modeliam.UserStatusActive,
		}
		require.NoError(t, fx.DB.Create(rootMember).Error)

		ctx := context.Background()
		ctx = reqctx.WithUserID(ctx, rootUser.ID)
		ctx = reqctx.WithTenantUUID(ctx, tenantB.UUID.String()) // non-membership but existing tenant

		resp, err := fx.Me.GetMeContext(ctx)
		require.NoError(t, err)
		require.True(t, resp.IsRoot)
		require.Equal(t, tenantB.UUID.String(), resp.CurrentTenantUUID)
		require.Nil(t, resp.CurrentMemberID, "root on non-membership tenant should not carry stale member id")
	})
}
