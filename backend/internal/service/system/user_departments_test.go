package system

import (
	"context"
	"testing"

	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repoi "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUserDepartmentsReadFilterUpdateAndTenantIsolation(t *testing.T) {
	db := newUserServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&m.Department{}, &m.DepartmentClosure{}, &m.MemberDepartment{}))
	ctx := context.Background()
	tenant, foreignTenant := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Create(&m.Role{Scope: "tenant", TenantUUID: tenant, Code: "role_user", Name: "Tenant User"}).Error)
	org := iamsvc.NewOrgService(db)
	parent := m.Department{TenantUUID: tenant, Name: "Parent", Key: "parent", Status: 1}
	require.NoError(t, org.CreateDepartment(ctx, &parent, nil))
	child := m.Department{TenantUUID: tenant, Name: "Child", Key: "child", Status: 1, ParentID: &parent.ID}
	require.NoError(t, org.CreateDepartment(ctx, &child, &parent.ID))
	foreign := m.Department{TenantUUID: foreignTenant, Name: "Foreign", Key: "foreign", Status: 1}
	require.NoError(t, org.CreateDepartment(ctx, &foreign, nil))
	svc := NewUserService(db)
	makeUser := func(name string, departments []uint64) uint64 {
		t.Helper()
		userUUID, err := svc.CreateSystemUser(ctx, &m.User{Email: name + "@example.invalid", DisplayName: name, Status: 1}, tenant, name, "", departments, nil)
		require.NoError(t, err)
		id, err := svc.ResolveUserIDByUUID(ctx, userUUID)
		require.NoError(t, err)
		return id
	}
	first := makeUser("first", []uint64{parent.ID, child.ID})
	second := makeUser("second", []uint64{child.ID})
	makeUser("unassigned", nil)
	read := func(recursive bool, page int) ([]iamsvc.MemberWithProfile, int64) {
		t.Helper()
		rows, total, err := svc.ListUsers(ctx, repoi.UserListFilter{TenantUUID: tenant, DeptID: &parent.ID, Recursive: recursive, Page: page, Size: 1, OrderBy: "id asc"})
		require.NoError(t, err)
		return rows, total
	}
	rows, total := read(true, 1)
	require.EqualValues(t, 2, total, "多部门成员不能重复计数")
	require.Len(t, rows, 1)
	require.Equal(t, first, rows[0].User.ID)
	require.ElementsMatch(t, []uint64{parent.ID, child.ID}, rows[0].DeptIDs)
	rows, total = read(true, 2)
	require.EqualValues(t, 2, total)
	require.Equal(t, second, rows[0].User.ID)
	rows, total = read(false, 1)
	require.EqualValues(t, 1, total)
	require.Equal(t, first, rows[0].User.ID)

	// 未提交部门字段保留多部门；跨租户失败必须回滚同时提交的姓名。
	require.NoError(t, svc.UpdateUserInTenant(ctx, first, tenant, map[string]any{"display_name": "Renamed"}, nil, nil))
	rows, _ = read(false, 1)
	require.ElementsMatch(t, []uint64{parent.ID, child.ID}, rows[0].DeptIDs)
	bad := []uint64{foreign.ID}
	require.Error(t, svc.UpdateUserInTenant(ctx, first, tenant, map[string]any{"display_name": "Must rollback"}, nil, &bad))
	rows, _ = read(false, 1)
	require.Equal(t, "Renamed", rows[0].User.DisplayName)
	require.ElementsMatch(t, []uint64{parent.ID, child.ID}, rows[0].DeptIDs)

	changed := []uint64{child.ID}
	require.NoError(t, svc.UpdateUserInTenant(ctx, first, tenant, nil, nil, &changed))
	_, total = read(false, 1)
	require.Zero(t, total)
	empty := []uint64{}
	require.NoError(t, svc.UpdateUserInTenant(ctx, first, tenant, nil, nil, &empty))
	rows, total = read(true, 1)
	require.EqualValues(t, 1, total)
	require.Equal(t, second, rows[0].User.ID)

	// 同样的校验必须覆盖租户成员更新与平台创建，不留下半成品账户。
	var member m.Member
	require.NoError(t, db.Where("tenant_uuid = ? AND user_id = ?", tenant, second).First(&member).Error)
	require.Error(t, iamsvc.NewMemberService(db).UpdateMember(ctx, tenant, member.ID, iamsvc.UpdateMemberInput{DeptIDs: &bad}))
	_, err := svc.CreateSystemUser(ctx, &m.User{Email: "rollback@example.invalid", DisplayName: "Rollback", Status: 1}, tenant, "rollback", "", bad, nil)
	require.Error(t, err)
	var count int64
	require.NoError(t, db.Model(&m.User{}).Where("email = ?", "rollback@example.invalid").Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Delete(&child).Error)
	require.Error(t, iamsvc.NewMemberService(db).PutMemberDepartments(ctx, tenant, member.ID, []uint64{child.ID}), "软删除部门不得重新关联")
}
