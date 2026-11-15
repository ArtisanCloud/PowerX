package iam

import (
	"context"
	"errors"
	"testing"
	"time"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type roleManagementFixture struct {
	db         *gorm.DB
	ctx        context.Context
	actor      ActorContext
	role       m.Role
	member     m.Member
	binding    m.RoleBinding
	permission m.Permission
}

func newRoleManagementFixture(t *testing.T) roleManagementFixture {
	t.Helper()
	db := setupRBACServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&m.User{}, &m.Member{}, &m.RoleBinding{}, &m.MemberAssignment{}))
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	tenant := uuid.NewString()
	user := m.User{Email: "fixture@example.invalid", Status: 1}
	require.NoError(t, db.Create(&user).Error)
	member := m.Member{TenantUUID: tenant, UserID: user.ID, UserUUID: user.UUID.String(), Username: "fixture", Status: 1}
	require.NoError(t, db.Create(&member).Error)
	role := m.Role{Scope: "tenant", TenantUUID: tenant, Code: "role_user", Name: "Tenant User"}
	require.NoError(t, db.Create(&role).Error)
	binding := m.RoleBinding{TenantUUID: tenant, RoleUUID: role.UUID.String(), RoleID: role.ID, SubjectType: m.SubMember, SubjectUUID: member.UUID.String(), SubjectID: member.ID}
	require.NoError(t, db.Create(&binding).Error)
	permission := m.Permission{Module: "platform_ops", Resource: "backup", Action: "execute", Source: "core", Status: "active", Effect: "allow"}
	require.NoError(t, db.Create(&permission).Error)
	// A stale/admin-looking role name in a valid session cannot confer authority.
	ctx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{Scope: "access", UserID: user.ID, UserUUID: user.UUID.String(), TenantUUID: tenant, MemberID: member.ID, MemberUUID: member.UUID.String(), Roles: []string{"role_admin", "system_admin"}})
	return roleManagementFixture{db, ctx, ActorContext{TenantUUID: tenant}, role, member, binding, permission}
}

func (f roleManagementFixture) grant(t *testing.T, action string) m.Permission {
	t.Helper()
	p := m.Permission{Module: "iam", Resource: "role", Action: action, Source: "core", Status: "active", Effect: "allow"}
	require.NoError(t, f.db.Create(&p).Error)
	require.NoError(t, f.db.Create(&m.RolePermission{RoleID: f.role.ID, RoleUUID: f.role.UUID, PermissionID: p.ID, PermissionUUID: p.PermissionUUID}).Error)
	return p
}

func TestIAM434OrdinaryMemberCannotEscalateOrMutateRoleState(t *testing.T) {
	f := newRoleManagementFixture(t)
	rbac, roles := NewRBACService(f.db), NewRoleService(f.db)
	scope := RoleContext{TenantUUID: f.actor.TenantUUID}
	checkBackup := func() {
		allowed, err := rbac.Enforce(f.ctx, f.actor, f.actor.TenantUUID, f.member.ID, "platform_ops", "backup", "execute")
		require.NoError(t, err)
		require.False(t, allowed)
	}
	checkBackup()
	for name, write := range map[string]func() error{
		"grant ids":   func() error { return rbac.GrantPermsByIDs(f.ctx, f.actor, f.role.ID, []uint64{f.permission.ID}) },
		"empty grant": func() error { return rbac.GrantPermsByIDs(f.ctx, f.actor, f.role.ID, nil) },
		"grant triples": func() error {
			return rbac.GrantPermsByTriples(f.ctx, f.actor, f.role.ID, []PermTriple{{"platform_ops", "backup", "execute"}})
		},
		"register arbitrary triple": func() error {
			return rbac.GrantPermsByTriples(f.ctx, f.actor, f.role.ID, []PermTriple{{"iam", "injected", "write"}})
		},
		"replace permissions": func() error {
			_, err := rbac.SetPermissionIDs(f.ctx, f.actor, f.role.ID, []uint64{f.permission.ID})
			return err
		},
		"revoke permissions": func() error {
			return rbac.RevokePermissionsFromRole(f.ctx, f.actor, f.role.ID, []uint64{f.permission.ID})
		},
		"bind role":   func() error { return rbac.BindRoleToMember(f.ctx, f.actor, f.actor.TenantUUID, f.role.ID, f.member.ID) },
		"unbind role": func() error { return rbac.UnbindRoleFromMember(f.ctx, f.actor, f.actor.TenantUUID, f.binding.ID) },
		"create role": func() error {
			_, err := roles.Create(f.ctx, scope, &m.Role{Scope: "tenant", TenantUUID: f.actor.TenantUUID, Code: "injected", Name: "Injected"})
			return err
		},
		"create with permissions": func() error {
			_, _, err := roles.CreateWithPerms(f.ctx, scope, &m.Role{Scope: "tenant", TenantUUID: f.actor.TenantUUID, Code: "injected", Name: "Injected"}, []uint64{f.permission.ID})
			return err
		},
		"update role": func() error { return roles.Update(f.ctx, scope, f.role.ID, &m.Role{Name: "Hijacked"}) },
		"delete role": func() error { return roles.Delete(f.ctx, scope, f.role.ID) },
	} {
		t.Run(name, func(t *testing.T) { require.Equal(t, 403, dto.StatusCode(write())) })
	}
	for _, table := range []struct {
		model any
		count int64
	}{{&m.Role{}, 1}, {&m.Permission{}, 1}, {&m.RolePermission{}, 0}, {&m.RoleBinding{}, 1}} {
		var count int64
		require.NoError(t, f.db.Model(table.model).Count(&count).Error)
		require.Equal(t, table.count, count)
	}
	var role m.Role
	require.NoError(t, f.db.First(&role, f.role.ID).Error)
	require.Equal(t, "Tenant User", role.Name)
	checkBackup()
}

func TestIAM434UsesLiveOwnedPermissionsAndSeparatesWriteFromBind(t *testing.T) {
	f := newRoleManagementFixture(t)
	svc := NewRBACService(f.db)
	p := f.grant(t, "write")
	require.NoError(t, svc.GrantPermsByIDs(f.ctx, f.actor, f.role.ID, []uint64{f.permission.ID}))
	require.Equal(t, 403, dto.StatusCode(svc.BindRoleToMember(f.ctx, f.actor, f.actor.TenantUUID, f.role.ID, f.member.ID)))
	for _, values := range []map[string]any{{"source": "plugin:com.example.iam"}, {"source": "core", "status": "deprecated"}, {"status": "active", "effect": "deny"}} {
		require.NoError(t, f.db.Model(&p).Updates(values).Error)
		require.Equal(t, 403, dto.StatusCode(svc.GrantPermsByIDs(f.ctx, f.actor, f.role.ID, []uint64{f.permission.ID})))
	}
	require.NoError(t, f.db.Model(&p).Update("effect", "allow").Error)
	expired := time.Now().Add(-time.Minute).UnixMilli()
	require.NoError(t, f.db.Model(&f.binding).Update("end_at", expired).Error)
	require.Equal(t, 403, dto.StatusCode(svc.AuthorizeManagement(f.ctx, f.actor, "write")))
	require.NoError(t, f.db.Model(&f.binding).Update("end_at", nil).Error)
	require.NoError(t, svc.AuthorizeManagement(f.ctx, f.actor, "write"))
	require.NoError(t, f.db.Where("permission_id = ?", p.ID).Delete(&m.RolePermission{}).Error)
	require.Equal(t, 403, dto.StatusCode(svc.GrantPermsByIDs(f.ctx, f.actor, f.role.ID, []uint64{f.permission.ID})), "the same session must stop working immediately after revocation")
}

func TestIAM434TenantManagersCannotChangeForeignRolesOrGlobalCatalog(t *testing.T) {
	f := newRoleManagementFixture(t)
	f.grant(t, "write")
	f.grant(t, "bind")
	f.grant(t, "read")
	svc := NewRBACService(f.db)
	foreign := m.Role{Scope: "tenant", TenantUUID: uuid.NewString(), Code: "foreign", Name: "Foreign"}
	require.NoError(t, f.db.Create(&foreign).Error)
	for _, write := range []func() error{
		func() error { return svc.GrantPermsByIDs(f.ctx, f.actor, foreign.ID, []uint64{f.permission.ID}) },
		func() error {
			return svc.GrantPermsByTriples(f.ctx, f.actor, foreign.ID, []PermTriple{{"unknown", "catalog", "write"}})
		},
		func() error { _, err := svc.SetPermissionIDs(f.ctx, f.actor, foreign.ID, nil); return err },
		func() error { return svc.BindRoleToMember(f.ctx, f.actor, foreign.TenantUUID, foreign.ID, f.member.ID) },
		func() error { _, err := svc.ListPermsOfRole(f.ctx, f.actor, foreign.ID); return err },
	} {
		require.Equal(t, 403, dto.StatusCode(write()))
	}
	require.Equal(t, 400, dto.StatusCode(svc.GrantPermsByTriples(f.ctx, f.actor, f.role.ID, []PermTriple{{"unknown", "catalog", "write"}})))
	var count int64
	require.NoError(t, f.db.Model(&m.Permission{}).Where("module = ?", "unknown").Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, svc.GrantPermsByTriples(f.ctx, f.actor, f.role.ID, []PermTriple{{"platform_ops", "backup", "execute"}, {"platform_ops", "backup", "execute"}}))
}

func TestIAM434RejectsServiceIdentityAndSpoofedRootActor(t *testing.T) {
	f := newRoleManagementFixture(t)
	svc := NewRBACService(f.db)
	require.Equal(t, 403, dto.StatusCode(svc.AuthorizeManagement(f.ctx, ActorContext{IsRoot: true}, "write")))
	for _, claims := range []*reqctx.CoreXClaims{nil, {UserID: 1, IsRoot: true, Scope: "refresh"}, {UserID: 1, IsRoot: true, Scope: "access", PluginID: "com.example.plugin"}, {UserID: 1, IsRoot: true, Scope: "access", Platforms: []string{"service_actor"}}} {
		ctx := reqctx.WithClaims(context.Background(), claims)
		require.Equal(t, 401, dto.StatusCode(svc.AuthorizeManagement(ctx, ActorContext{IsRoot: true}, "write")))
	}
	ctx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access"})
	require.NoError(t, svc.GrantPermsByIDs(ctx, ActorContext{IsRoot: true}, f.role.ID, []uint64{f.permission.ID}))
}

func TestIAM434DedicatedPermissionSurvivesLegacyPluginCollision(t *testing.T) {
	f := newRoleManagementFixture(t)
	legacy := f.grant(t, "write")
	require.NoError(t, f.db.Model(&legacy).Update("source", "plugin:com.powerx.plugins.ai-craft").Error)
	svc := NewRBACService(f.db)
	require.Equal(t, 403, dto.StatusCode(svc.AuthorizeManagement(f.ctx, f.actor, "write")))
	p := m.Permission{Module: "iam", Resource: "role_management", Action: "write", Source: "platform_capability", Status: "active", Effect: "allow"}
	require.NoError(t, f.db.Create(&p).Error)
	require.NoError(t, f.db.Create(&m.RolePermission{RoleID: f.role.ID, RoleUUID: f.role.UUID, PermissionID: p.ID, PermissionUUID: p.PermissionUUID}).Error)
	require.NoError(t, svc.AuthorizeManagement(f.ctx, f.actor, "write"))
	require.NoError(t, f.db.Model(&p).Update("source", "plugin:com.example.impostor").Error)
	require.Equal(t, 403, dto.StatusCode(svc.AuthorizeManagement(f.ctx, f.actor, "write")))
	require.NoError(t, f.db.First(&legacy, legacy.ID).Error)
	require.Equal(t, "plugin:com.powerx.plugins.ai-craft", legacy.Source)
}

func TestIAM434InitialPermissionFailureRollsBackRoleCreation(t *testing.T) {
	f := newRoleManagementFixture(t)
	f.grant(t, "write")
	failure := errors.New("isolated initial grant failure")
	require.NoError(t, f.db.Callback().Create().Before("gorm:create").Register("iam434:reject-grant", func(tx *gorm.DB) {
		if tx.Statement.Table == (&m.RolePermission{}).GetTableName(false) {
			tx.AddError(failure)
		}
	}))
	t.Cleanup(func() { _ = f.db.Callback().Create().Remove("iam434:reject-grant") })
	out, _, err := NewRoleService(f.db).CreateWithPerms(f.ctx, RoleContext{TenantUUID: f.actor.TenantUUID}, &m.Role{Scope: "tenant", TenantUUID: f.actor.TenantUUID, Code: "must_rollback", Name: "Must roll back"}, []uint64{f.permission.ID})
	require.ErrorIs(t, err, failure)
	require.Nil(t, out)
	var count int64
	require.NoError(t, f.db.Model(&m.Role{}).Where("code = ?", "must_rollback").Count(&count).Error)
	require.Zero(t, count)
}

func TestIAM434DedicatedBindPermissionWorksForBothMemberAssignmentEntrypoints(t *testing.T) {
	f := newRoleManagementFixture(t)
	p := m.Permission{Module: "iam", Resource: "role_management", Action: "bind", Source: "platform_capability", Status: "active", Effect: "allow"}
	require.NoError(t, f.db.Create(&p).Error)
	require.NoError(t, f.db.Create(&m.RolePermission{RoleID: f.role.ID, RoleUUID: f.role.UUID, PermissionID: p.ID, PermissionUUID: p.PermissionUUID}).Error)
	role := m.Role{Scope: "tenant", TenantUUID: f.actor.TenantUUID, Code: "assigned", Name: "Assigned"}
	require.NoError(t, f.db.Create(&role).Error)
	require.NoError(t, NewRBACService(f.db).BindRoleToMember(f.ctx, f.actor, f.actor.TenantUUID, role.ID, f.member.ID))
	require.NoError(t, NewMemberService(f.db).authorizeMemberAccountPermission(f.ctx, "role", "bind"))
}
