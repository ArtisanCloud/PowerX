package iam

import (
	"context"
	"testing"
	"time"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestMemberManagementUsesOwnedLiveUUIDPermissions(t *testing.T) {
	db := setupRBACServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&m.RoleBinding{}, &m.MemberAssignment{}))
	tenant, memberUUID := uuid.NewString(), uuid.NewString()
	role := m.Role{Scope: "tenant", TenantUUID: tenant, Code: "role_admin", Name: "Tenant Admin"}
	require.NoError(t, db.Create(&role).Error)
	binding := m.RoleBinding{TenantUUID: tenant, RoleUUID: role.UUID.String(), RoleID: 999, SubjectType: m.SubMember, SubjectUUID: memberUUID, SubjectID: 999}
	require.NoError(t, db.Create(&binding).Error)
	ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant), &reqctx.CoreXClaims{TenantUUID: tenant, MemberID: 7, MemberUUID: memberUUID, Roles: []string{"role_admin"}})
	svc := NewMemberService(db)
	legacy := m.Permission{Module: "iam", Resource: "user", Action: "read", Source: "plugin:com.example.plugin", Status: "deprecated", Effect: "allow"}
	require.NoError(t, db.Create(&legacy).Error)
	require.NoError(t, db.Create(&m.RolePermission{RoleUUID: role.UUID, RoleID: role.ID, PermissionUUID: legacy.PermissionUUID, PermissionID: legacy.ID}).Error)
	require.Equal(t, 403, dto.StatusCode(svc.AuthorizeManagement(ctx, "read")), "旧插件权限与角色名称不能放行底座管理")
	permission := m.Permission{Module: "iam", Resource: "member_accounts", Action: "read", Source: "platform_capability", Status: "active", Effect: "allow"}
	require.NoError(t, db.Create(&permission).Error)
	grant := m.RolePermission{RoleUUID: role.UUID, RoleID: 998, PermissionUUID: permission.PermissionUUID, PermissionID: 998}
	require.NoError(t, db.Create(&grant).Error)
	check := func(allowed bool) {
		t.Helper()
		err := svc.AuthorizeManagement(ctx, "read")
		if allowed {
			require.NoError(t, err)
		} else {
			require.Equal(t, 403, dto.StatusCode(err))
		}
	}
	check(true) // UUID 关系有效，不依赖陈旧数字列。
	for _, values := range []map[string]any{{"source": "plugin:com.example.plugin"}, {"source": "platform_capability", "status": "deprecated"}, {"status": "active", "effect": "deny"}} {
		require.NoError(t, db.Model(&permission).Updates(values).Error)
		check(false)
	}
	require.NoError(t, db.Model(&permission).Update("effect", "allow").Error)
	check(true)
	expired := time.Now().Add(-time.Minute).UnixMilli()
	require.NoError(t, db.Model(&binding).Update("end_at", expired).Error)
	check(false)
	require.NoError(t, db.Model(&binding).Update("end_at", nil).Error)
	check(true)
	require.NoError(t, db.Where("role_uuid = ? AND permission_uuid = ?", role.UUID, permission.PermissionUUID).Delete(&m.RolePermission{}).Error)
	check(false)
}
