package authorizationbaseline

import (
	"context"
	"errors"
	"testing"
	"time"

	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	sm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	ar "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/authorization"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/rootrole"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRootSourcesShareSnapshotAndLedgerWithLiveIdentity(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.db.Delete(&f.binding).Error)
	_, denied := f.current(t)
	require.Equal(t, Denied, denied.State)
	before := Micro(time.Now())
	require.NoError(t, f.db.Model(&im.User{}).Where("uuid=?", f.member.UserUUID).Update("is_root", true).Error)
	_, first := f.current(t)
	require.Equal(t, Allowed, first.State)
	require.Equal(t, "native", first.BaselineKind)
	require.NotNil(t, first.GrantedAt)
	require.False(t, first.GrantedAt.Before(before))
	facts, err := ar.NewRepository(f.db).Load(context.Background(), f.tenant, f.plugin)
	require.NoError(t, err)
	sources, root := MemberSources(facts, f.member, "work.item:read")
	require.True(t, root)
	require.Len(t, sources, 1)
	require.Contains(t, sources[0].ID, rootSourcePrefix)
	roles, permissions := SnapshotPermissions(facts, f.member.UUID.String(), f.member.ID, time.Now())
	require.Equal(t, []string{"root"}, roles)
	require.Len(t, permissions, 1)
	require.Equal(t, f.permission.PermissionUUID, permissions[0].PermissionUUID)
	var bindingCount int64
	require.NoError(t, f.db.Model(&im.RoleBinding{}).Count(&bindingCount).Error)
	require.Zero(t, bindingCount)
	// A different member or a fabricated member identifier cannot inherit this source.
	require.Empty(t, Grants(facts, uuid.NewString(), f.member.ID))
	require.Empty(t, Grants(facts, f.member.UUID.String(), f.member.ID+1))
	// A rollback cannot publish either a changed identity or ledger segment.
	err = f.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&im.User{}).Where("uuid=?", f.member.UserUUID).Update("is_root", false).Error; err != nil {
			return err
		}
		return errors.New("abort")
	})
	require.EqualError(t, err, "abort")
	_, unchanged := f.current(t)
	require.Equal(t, first, unchanged)
	// Every supported lifecycle change closes the root source in the same transaction.
	for _, subject := range []struct {
		name    string
		model   any
		where   string
		value   any
		field   string
		off, on any
	}{
		{"member", &im.Member{}, "uuid=?", f.member.UUID, "status", 2, 1},
		{"user", &im.User{}, "uuid=?", f.member.UserUUID, "status", 2, 1},
		{"tenant", &tm.Tenant{}, "uuid=?", f.tenant, "status", 2, 1},
		{"plugin", &sm.PluginInstanceConfig{}, "tenant_uuid=?", f.tenant, "enabled", false, true},
		{"root identity", &im.User{}, "uuid=?", f.member.UserUUID, "is_root", false, true},
	} {
		t.Run(subject.name, func(t *testing.T) {
			require.NoError(t, f.db.Model(subject.model).Where(subject.where, subject.value).Update(subject.field, subject.off).Error)
			_, closed := f.current(t)
			require.Equal(t, Denied, closed.State)
			require.NoError(t, f.db.Model(subject.model).Where(subject.where, subject.value).Update(subject.field, subject.on).Error)
			_, reopened := f.current(t)
			require.Equal(t, Allowed, reopened.State)
			require.NotEqual(t, first.EpochUUID, reopened.EpochUUID)
			first = reopened
		})
	}
}

func TestRootRoleCannotBeGrantedByFabricatedBindingOrMissingIdentity(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.db.Delete(&f.binding).Error)
	var root im.Role
	require.NoError(t, f.db.Where("scope=? AND code=?", "system", iam.CodeSystemRoot).Take(&root).Error)
	forged := im.RoleBinding{TenantUUID: f.tenant, RoleID: root.ID, RoleUUID: root.UUID.String(), SubjectType: im.SubMember, SubjectID: f.member.ID, SubjectUUID: f.member.UUID.String(), DataScope: im.ScopeTenant}
	require.NoError(t, f.db.Create(&forged).Error)
	_, denied := f.current(t)
	require.Equal(t, Denied, denied.State, "physical binding cannot manufacture root identity")
	require.NoError(t, f.db.Model(&im.User{}).Where("uuid=?", f.member.UserUUID).Update("is_root", true).Error)
	facts, err := ar.NewRepository(f.db).Load(context.Background(), f.tenant, f.plugin)
	require.NoError(t, err)
	require.Len(t, Grants(facts, f.member.UUID.String(), f.member.ID), 1)
	for _, mutate := range []func(*ar.Facts){
		func(f *ar.Facts) { f.Users = nil },
		func(f *ar.Facts) { f.Members = nil },
		func(f *ar.Facts) { f.Users[0].ID++ },
		func(f *ar.Facts) { f.RolePermissions = nil },
		func(f *ar.Facts) {
			for i := range f.Roles {
				if rootrole.IsManaged(f.Roles[i]) {
					f.Roles[i].Builtin = false
				}
			}
		},
	} {
		copyFacts, err := ar.NewRepository(f.db).Load(context.Background(), f.tenant, f.plugin)
		require.NoError(t, err)
		mutate(&copyFacts)
		require.Empty(t, Grants(copyFacts, f.member.UUID.String(), f.member.ID))
	}
}

func TestLegacyRootUpgradePreservesHistoryAndDoesNotHealUntrackedDrift(t *testing.T) {
	for _, repairCatalog := range []bool{false, true} {
		t.Run(map[bool]string{false: "existing catalog", true: "repair missing catalog"}[repairCatalog], func(t *testing.T) {
			testLegacyRootUpgrade(t, repairCatalog)
		})
	}
}

func testLegacyRootUpgrade(t *testing.T, repairCatalog bool) {
	db := bootstrapDB(t)
	plugin := "com.example.orders"
	p := bootstrapPermission(t, db, plugin, "item")
	rootMember := bootstrapTenant(t, db, plugin, p, false)
	ordinary := bootstrapTenant(t, db, plugin, p, false)
	require.NoError(t, db.Model(&im.User{}).Where("uuid=?", rootMember.member.UserUUID).Update("is_root", true).Error)
	past := Micro(time.Now().Add(-24 * time.Hour))
	for _, f := range []fixture{rootMember, ordinary} {
		require.NoError(t, db.Create(&im.AuthorizationTracking{TenantUUID: f.tenant, PluginID: plugin, PermissionCode: "work.item:read", Status: "ready", KnownSince: &past}).Error)
	}
	// Old algorithm saw no managed role source, even if its permission catalog existed.
	require.NoError(t, Reconcile(db, past))
	rootBefore, old := rootMember.current(t)
	require.Equal(t, RootBypass, old.State)
	ordinaryBefore, ordinarySegment := ordinary.current(t)
	_, err := rootrole.Ensure(db)
	require.NoError(t, err)
	var rootRole im.Role
	require.NoError(t, db.Where("scope=? AND code=?", "system", iam.CodeSystemRoot).Take(&rootRole).Error)
	// An unrelated offline grant has unknown timing; startup must not bless that drift.
	role := im.Role{TenantUUID: ordinary.tenant, Scope: "tenant", Code: "role_admin", Builtin: true}
	require.NoError(t, db.Create(&role).Error)
	require.NoError(t, db.Create(&im.RolePermission{RoleID: role.ID, RoleUUID: role.UUID, PermissionID: p.ID, PermissionUUID: p.PermissionUUID}).Error)
	require.NoError(t, db.Create(&im.RoleBinding{TenantUUID: ordinary.tenant, RoleID: role.ID, RoleUUID: role.UUID.String(), SubjectType: im.SubMember, SubjectID: ordinary.member.ID, SubjectUUID: ordinary.member.UUID.String(), DataScope: im.ScopeTenant}).Error)
	if repairCatalog {
		require.NoError(t, db.Where("role_uuid=?", rootRole.UUID).Delete(&im.RolePermission{}).Error)
	}
	require.NoError(t, Install(db))
	before := Micro(time.Now())
	require.NoError(t, EnsureInitialized(db))
	after := Micro(time.Now())
	rootAfter, upgraded := rootMember.current(t)
	require.Equal(t, rootBefore.UUID, rootAfter.UUID)
	require.Equal(t, rootBefore.KnownSince, rootAfter.KnownSince)
	require.Equal(t, rootBefore.FactRevision+1, rootAfter.FactRevision)
	require.Equal(t, Allowed, upgraded.State)
	require.Equal(t, "native", upgraded.BaselineKind)
	require.NotNil(t, upgraded.GrantedAt)
	require.False(t, upgraded.GrantedAt.Before(before))
	require.False(t, upgraded.GrantedAt.After(after))
	var historical im.AuthorizationSegment
	require.NoError(t, db.Where("uuid=?", old.UUID).Take(&historical).Error)
	require.Equal(t, old.State, historical.State)
	require.Equal(t, old.EffectiveFrom, historical.EffectiveFrom)
	require.Equal(t, upgraded.EffectiveFrom, *historical.EffectiveUntil)
	untracked, sameSegment := ordinary.current(t)
	require.Equal(t, ordinaryBefore, untracked)
	require.Equal(t, ordinarySegment, sameSegment)
	// Idempotence includes catalog identity, streams, history, and member/binding counts.
	var streams []im.AuthorizationStream
	var segments []im.AuthorizationSegment
	require.NoError(t, db.Order("id").Find(&streams).Error)
	require.NoError(t, db.Order("id").Find(&segments).Error)
	require.NoError(t, EnsureInitialized(db))
	var streamsAgain []im.AuthorizationStream
	var segmentsAgain []im.AuthorizationSegment
	require.NoError(t, db.Order("id").Find(&streamsAgain).Error)
	require.NoError(t, db.Order("id").Find(&segmentsAgain).Error)
	require.Equal(t, streams, streamsAgain)
	require.Equal(t, segments, segmentsAgain)
	var restoredRole im.Role
	require.NoError(t, db.First(&restoredRole, rootRole.ID).Error)
	require.Equal(t, rootRole, restoredRole)
	var members, bindings int64
	require.NoError(t, db.Model(&im.Member{}).Count(&members).Error)
	require.NoError(t, db.Model(&im.RoleBinding{}).Count(&bindings).Error)
	require.EqualValues(t, 2, members)
	require.EqualValues(t, 1, bindings)
}
