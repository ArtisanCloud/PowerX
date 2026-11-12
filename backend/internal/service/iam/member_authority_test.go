package iam

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	host "github.com/ArtisanCloud/PowerX/internal/testutil/runtime_host"
	am "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func authorityFixture(t *testing.T) (*gorm.DB, context.Context, context.Context, *setting.PluginInstanceConfig, []im.Member) {
	t.Helper()
	db := host.Database(t)
	require.NoError(t, db.AutoMigrate(&im.User{}, &im.Member{}, &im.Role{}, &im.RoleBinding{}, &im.Department{}, &im.MemberDepartment{}, &am.AuditEvent{}))
	tenant := uuid.NewString()
	service, cred := host.Actor(t, db, tenant, "plugin.organization.test")
	require.NoError(t, db.Create(&cm.CapabilityRecord{CapabilityID: MemberAuthorityCapability, PluginID: "core", PluginVersion: "1", Status: "published"}).Error)
	require.NoError(t, db.Create(&cm.CapabilityRegistration{CapabilityID: MemberAuthorityCapability, TenantUUID: tenant, Status: "published", Version: 1, RoutingPolicyID: uuid.New()}).Error)
	host.Grant(t, db, cred, MemberAuthorityCapability)
	members := []im.Member{}
	for _, name := range []string{"reporter", "manager", "director"} {
		u := im.User{Email: name + "@organization.test", Status: 1}
		require.NoError(t, db.Create(&u).Error)
		m := im.Member{TenantUUID: tenant, UserID: u.ID, UserUUID: u.UUID.String(), Username: name, Status: 1}
		require.NoError(t, db.Create(&m).Error)
		require.NoError(t, db.First(&m, m.ID).Error)
		members = append(members, m)
	}
	root := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant), &reqctx.CoreXClaims{TenantUUID: tenant, MemberUUID: members[0].UUID.String(), UserUUID: members[0].UserUUID, IsRoot: true})
	return db, service, root, cred, members
}

func TestMemberAuthorityUsesUUIDRoleAndDepartmentRelationsAndExplicitManager(t *testing.T) {
	db, ctx, root, _, members := authorityFixture(t)
	tenant := reqctx.GetTenantUUID(ctx)
	role := im.Role{TenantUUID: tenant, Code: "supervisor", Name: "fixture_role"}
	require.NoError(t, db.Create(&role).Error)
	require.NoError(t, db.Create(&im.RoleBinding{TenantUUID: tenant, RoleUUID: role.UUID.String(), RoleID: 999, SubjectType: im.SubMember, SubjectUUID: members[1].UUID.String(), SubjectID: 999}).Error)
	parent := im.Department{TenantUUID: tenant, Key: "parent", Name: "fixture_parent", Status: 1, LeaderMemberUUID: &members[2].UUID}
	require.NoError(t, db.Create(&parent).Error)
	child := im.Department{TenantUUID: tenant, Key: "child", Name: "fixture_child", Status: 1, ParentDepartmentUUID: &parent.DepartmentUUID, LeaderMemberUUID: &members[1].UUID}
	require.NoError(t, db.Create(&child).Error)
	require.NoError(t, db.Create(&im.MemberDepartment{TenantUUID: tenant, MemberID: 999, DepartmentID: 999, MemberUUID: members[0].UUID, DepartmentUUID: child.DepartmentUUID}).Error)
	query := MemberAuthorityQuery{Schema: "powerx.iam.member-authority-query.v1", MemberUUIDs: []string{members[0].UUID.String(), members[1].UUID.String(), uuid.NewString()}}
	before, err := NewTenantDirectoryService(db).ReadMemberAuthority(ctx, query)
	require.NoError(t, err)
	require.Equal(t, "unset", before.Members[0].ManagerStatus)
	require.Nil(t, before.Members[0].DirectManagerMemberUUID)
	require.Equal(t, []string{"supervisor"}, before.Members[1].RoleCodes)
	require.Equal(t, "not_found", before.Members[2].Status)
	require.Len(t, before.Departments, 2)
	manager := members[1].UUID.String()
	require.NoError(t, NewMemberService(db).SetReportingManager(root, tenant, members[0].UUID.String(), ReportingManagerInput{ManagerMemberUUID: &manager, ExpectedUpdatedAt: members[0].UpdatedAt, Reason: "fixture_manager_basis"}))
	after, err := NewTenantDirectoryService(db).ReadMemberAuthority(ctx, query)
	require.NoError(t, err)
	require.Equal(t, "active", after.Members[0].ManagerStatus)
	require.Equal(t, manager, *after.Members[0].DirectManagerMemberUUID)
	var count int64
	require.NoError(t, db.Model(&am.AuditEvent{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, db.Model(&im.Member{}).Where("uuid = ?", manager).Update("status", 0).Error)
	after, err = NewTenantDirectoryService(db).ReadMemberAuthority(ctx, query)
	require.NoError(t, err)
	require.Equal(t, "unavailable", after.Members[0].ManagerStatus)
	require.Nil(t, after.Members[0].DirectManagerMemberUUID)
}

func TestReportingManagerRejectsCycleStaleVersionAndServiceWriter(t *testing.T) {
	db, service, root, _, members := authorityFixture(t)
	tenant := reqctx.GetTenantUUID(root)
	manager := members[1].UUID.String()
	s := NewMemberService(db)
	input := ReportingManagerInput{ManagerMemberUUID: &manager, ExpectedUpdatedAt: members[0].UpdatedAt, Reason: "fixture_basis"}
	require.Error(t, s.SetReportingManager(service, tenant, members[0].UUID.String(), input))
	require.NoError(t, s.SetReportingManager(root, tenant, members[0].UUID.String(), input))
	var coded *dto.AppError
	require.ErrorAs(t, s.SetReportingManager(root, tenant, members[0].UUID.String(), input), &coded)
	require.Equal(t, "IAM_ORGANIZATION_CHANGED", coded.Code)
	reporter := members[0].UUID.String()
	require.ErrorAs(t, s.SetReportingManager(root, tenant, manager, ReportingManagerInput{ManagerMemberUUID: &reporter, ExpectedUpdatedAt: members[1].UpdatedAt, Reason: "fixture_cycle"}), &coded)
	require.Equal(t, "IAM_ORGANIZATION_CYCLE", coded.Code)
	current := im.Member{}
	require.NoError(t, db.Where("uuid = ?", reporter).Take(&current).Error)
	require.NoError(t, s.SetReportingManager(root, tenant, reporter, ReportingManagerInput{ExpectedUpdatedAt: current.UpdatedAt, Reason: "fixture_clear"}))
	current = im.Member{}
	require.NoError(t, db.Where("uuid = ?", reporter).Take(&current).Error)
	require.Nil(t, current.DirectManagerMemberUUID)
}

func TestReportingManagerAuditFailureRollsBackRelationAndTimestamp(t *testing.T) {
	db, _, root, _, members := authorityFixture(t)
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("fixture_audit_abort", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == (&am.AuditEvent{}).TableName() {
			tx.AddError(errors.New("fixture_audit_failure"))
		}
	}))
	defer db.Callback().Create().Remove("fixture_audit_abort")
	manager := members[1].UUID.String()
	require.ErrorContains(t, NewMemberService(db).SetReportingManager(root, reqctx.GetTenantUUID(root), members[0].UUID.String(), ReportingManagerInput{ManagerMemberUUID: &manager, ExpectedUpdatedAt: members[0].UpdatedAt, Reason: "fixture_basis"}), "fixture_audit_failure")
	var current im.Member
	require.NoError(t, db.Where("uuid = ?", members[0].UUID).Take(&current).Error)
	require.Nil(t, current.DirectManagerMemberUUID)
	require.True(t, current.UpdatedAt.Equal(members[0].UpdatedAt))
}

func TestMemberAuthorityIsBoundedGrantCheckedAndDoesNotUseLegacyNumericRoleFallback(t *testing.T) {
	db, ctx, root, cred, members := authorityFixture(t)
	s := NewTenantDirectoryService(db)
	query := MemberAuthorityQuery{Schema: "powerx.iam.member-authority-query.v1", MemberUUIDs: []string{members[0].UUID.String()}}
	_, err := s.ReadMemberAuthority(root, query)
	require.Error(t, err)
	host.Grant(t, db, cred)
	_, err = s.ReadMemberAuthority(ctx, query)
	require.Error(t, err)
	host.Grant(t, db, cred, MemberAuthorityCapability)
	invalid := query
	invalid.MemberUUIDs = []string{members[0].UUID.String(), members[0].UUID.String()}
	_, err = s.ReadMemberAuthority(ctx, invalid)
	require.Error(t, err)
	require.NoError(t, db.Create(&im.RoleBinding{TenantUUID: reqctx.GetTenantUUID(ctx), RoleID: 999, SubjectType: im.SubMember, SubjectUUID: members[0].UUID.String(), SubjectID: members[0].ID}).Error)
	_, err = s.ReadMemberAuthority(ctx, query)
	require.Error(t, err)
}

func TestReportingManagerPostgresConcurrentCycleCannotCommit(t *testing.T) {
	if os.Getenv("POWERX_RUNTIME_HOST_TEST_POSTGRES_DSN") == "" {
		t.Skip("POWERX_RUNTIME_HOST_TEST_POSTGRES_DSN required")
	}
	db, _, root, _, members := authorityFixture(t)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			manager := members[1-i].UUID.String()
			results <- NewMemberService(db).SetReportingManager(root, reqctx.GetTenantUUID(root), members[i].UUID.String(), ReportingManagerInput{ManagerMemberUUID: &manager, ExpectedUpdatedAt: members[i].UpdatedAt, Reason: "fixture_concurrent"})
		}(i)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	require.Equal(t, 1, success)
	var n int64
	require.NoError(t, db.Model(&am.AuditEvent{}).Where("occurred_at <= ?", time.Now().Add(time.Minute)).Count(&n).Error)
	require.EqualValues(t, 1, n)
}
