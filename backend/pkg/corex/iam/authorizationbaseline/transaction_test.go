package authorizationbaseline

import (
	"errors"
	"testing"
	"time"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	sm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type fixture struct {
	db             *gorm.DB
	tenant, plugin string
	member         im.Member
	role           im.Role
	permission     im.Permission
	binding        im.RoleBinding
	at             time.Time
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	old := model.PowerXSchema
	model.PowerXSchema = "main"
	t.Cleanup(func() { model.PowerXSchema = old })
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&im.AuthorizationTracking{}, &im.AuthorizationStream{}, &im.AuthorizationSegment{}, &im.User{}, &im.Member{}, &im.Role{}, &im.Permission{}, &im.RolePermission{}, &im.RoleBinding{}, &im.MemberAssignment{}, &tm.Tenant{}, &sm.PluginInstanceConfig{}))
	return seedFixture(t, db)
}

func seedFixture(t *testing.T, db *gorm.DB) fixture {
	f := fixture{db: db, tenant: uuid.NewString(), plugin: "com.example.baseline", at: Micro(time.Now().Add(-time.Hour))}
	user := im.User{Email: uuid.NewString() + "@example.invalid", Status: 1}
	require.NoError(t, db.Create(&user).Error)
	f.member = im.Member{TenantUUID: f.tenant, UserID: user.ID, UserUUID: user.UUID.String(), Username: "member", Status: 1}
	require.NoError(t, db.Create(&f.member).Error)
	tenant := tm.Tenant{Key: uuid.NewString(), Status: 1}
	tenant.UUID = uuid.MustParse(f.tenant)
	require.NoError(t, db.Create(&tenant).Error)
	require.NoError(t, db.Create(&sm.PluginInstanceConfig{TenantUUID: f.tenant, PluginID: f.plugin, Key: "auth.credentials", Enabled: true}).Error)
	f.role = im.Role{TenantUUID: f.tenant, Scope: "tenant", Code: "reader"}
	require.NoError(t, db.Create(&f.role).Error)
	f.permission = im.Permission{Module: "work", Resource: "item", Action: "read", Effect: "allow", Status: im.PermissionStatusActive, Source: "plugin:" + f.plugin, Meta: datatypes.JSON(`{"authorization_baseline":true,"data_scope":"TENANT"}`)}
	require.NoError(t, db.Create(&f.permission).Error)
	require.NoError(t, db.Create(&im.RolePermission{RoleID: f.role.ID, RoleUUID: f.role.UUID, PermissionID: f.permission.ID, PermissionUUID: f.permission.PermissionUUID}).Error)
	f.binding = im.RoleBinding{TenantUUID: f.tenant, RoleID: f.role.ID, RoleUUID: f.role.UUID.String(), SubjectType: im.SubMember, SubjectID: f.member.ID, SubjectUUID: f.member.UUID.String(), DataScope: im.ScopeTenant}
	require.NoError(t, db.Create(&f.binding).Error)
	require.NoError(t, Install(db))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := Lock(tx); err != nil {
			return err
		}
		track := im.AuthorizationTracking{TenantUUID: f.tenant, PluginID: f.plugin, PermissionCode: "work.item:read", Status: "ready", KnownSince: &f.at}
		if err := tx.Create(&track).Error; err != nil {
			return err
		}
		return Reconcile(tx, f.at)
	}))
	return f
}
func (f fixture) current(t *testing.T) (im.AuthorizationStream, im.AuthorizationSegment) {
	t.Helper()
	var stream im.AuthorizationStream
	require.NoError(t, f.db.Where("tenant_uuid=? AND member_uuid=?", f.tenant, f.member.UUID.String()).Take(&stream).Error)
	var segment im.AuthorizationSegment
	now := Micro(time.Now())
	require.NoError(t, f.db.Where("stream_uuid=? AND effective_from<=? AND (effective_until IS NULL OR effective_until>?)", stream.UUID.String(), now, now).Take(&segment).Error)
	return stream, segment
}
func TestTransactionOverlapRevokeRegrantNoopAndRollback(t *testing.T) {
	f := newFixture(t)
	stream, first := f.current(t)
	require.Equal(t, Allowed, first.State)
	require.Equal(t, "initialized", first.BaselineKind)
	require.Nil(t, first.GrantedAt)
	roleB := im.Role{TenantUUID: f.tenant, Scope: "tenant", Code: "reader_b"}
	require.NoError(t, f.db.Create(&roleB).Error)
	require.NoError(t, f.db.Create(&im.RolePermission{RoleID: roleB.ID, RoleUUID: roleB.UUID, PermissionID: f.permission.ID, PermissionUUID: f.permission.PermissionUUID}).Error)
	b := f.binding
	b.ID = 0
	b.RoleID = roleB.ID
	b.RoleUUID = roleB.UUID.String()
	require.NoError(t, f.db.Create(&b).Error)
	streamB, overlap := f.current(t)
	require.Equal(t, first.EpochUUID, overlap.EpochUUID)
	require.Greater(t, streamB.FactRevision, stream.FactRevision)
	require.NoError(t, f.db.Delete(&f.binding).Error)
	_, remaining := f.current(t)
	require.Equal(t, first.EpochUUID, remaining.EpochUUID)
	require.NoError(t, f.db.Delete(&b).Error)
	_, denied := f.current(t)
	require.Equal(t, Denied, denied.State)
	b.ID = 0
	require.NoError(t, f.db.Create(&b).Error)
	newStream, native := f.current(t)
	require.Equal(t, "native", native.BaselineKind)
	require.NotNil(t, native.GrantedAt)
	require.NotEqual(t, first.EpochUUID, native.EpochUUID)
	require.NoError(t, f.db.Model(&f.member).Update("display_name", "changed").Error)
	afterNoop, _ := f.current(t)
	require.Equal(t, newStream.FactRevision, afterNoop.FactRevision)
	err := f.db.Transaction(func(tx *gorm.DB) error {
		if e := tx.Delete(&b).Error; e != nil {
			return e
		}
		return errors.New("rollback")
	})
	require.Error(t, err)
	afterRollback, same := f.current(t)
	require.Equal(t, newStream.FactRevision, afterRollback.FactRevision)
	require.Equal(t, native.EpochUUID, same.EpochUUID)
	// 覆盖写入改变物理行 ID，但不能制造连续访问中断。
	require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error {
		if e := tx.Delete(&b).Error; e != nil {
			return e
		}
		b.ID = 0
		return tx.Create(&b).Error
	}))
	coveredStream, covered := f.current(t)
	require.Equal(t, native.EpochUUID, covered.EpochUUID)
	require.Equal(t, newStream.FactRevision, coveredStream.FactRevision)
}
func TestRawMutationAndDisabledAccountUseTheSameCommitBoundary(t *testing.T) {
	f := newFixture(t)
	_, first := f.current(t)
	require.NoError(t, f.db.Exec("UPDATE "+(&im.Member{}).TableName()+" SET status=? WHERE uuid=?", 2, f.member.UUID).Error)
	_, disabled := f.current(t)
	require.Equal(t, Denied, disabled.State)
	require.NoError(t, f.db.Model(&f.member).Update("status", 1).Error)
	_, enabled := f.current(t)
	require.NotEqual(t, first.EpochUUID, enabled.EpochUUID)
	// 跳过事务的写法不能悄悄丢掉维护。
	require.ErrorIs(t, f.db.Session(&gorm.Session{SkipDefaultTransaction: true}).Model(&f.member).Update("status", 2).Error, ErrWriteTransaction)
}
func TestMaintenanceFailureRollsBackTheOriginalSourceMutation(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.db.Exec("DROP TABLE "+(&im.AuthorizationSegment{}).TableName()).Error)
	require.Error(t, f.db.Model(&f.member).Update("status", 2).Error)
	var member im.Member
	require.NoError(t, f.db.First(&member, f.member.ID).Error)
	require.EqualValues(t, 1, member.Status)
}

func TestCommitMaintenanceFailureAndSavepointRollbackAreAtomic(t *testing.T) {
	f := newFixture(t)
	before, first := f.current(t)
	require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error {
		err := tx.Transaction(func(nested *gorm.DB) error {
			if err := nested.Delete(&f.binding).Error; err != nil {
				return err
			}
			return errors.New("rollback savepoint")
		})
		require.Error(t, err)
		return nil
	}))
	after, same := f.current(t)
	require.Equal(t, before.FactRevision, after.FactRevision)
	require.Equal(t, first.EpochUUID, same.EpochUUID)
	require.NoError(t, f.db.Callback().Create().Before("gorm:create").Register("test:fail_authorization_segment", func(tx *gorm.DB) {
		if tx.Statement.Table == "iam_authorization_segments" {
			tx.AddError(errors.New("ledger unavailable"))
		}
	}))
	require.Error(t, f.db.Model(&f.member).Update("status", 2).Error)
	var member im.Member
	require.NoError(t, f.db.First(&member, f.member.ID).Error)
	require.EqualValues(t, 1, member.Status)
}
func TestSavedFutureSegmentsSurviveNoQueriesAndNoWorker(t *testing.T) {
	f := newFixture(t)
	from := Micro(time.Now()).Add(time.Hour).Truncate(time.Millisecond)
	until := from.Add(time.Hour)
	startMS, endMS := from.UnixMilli(), until.UnixMilli()
	require.NoError(t, f.db.Model(&f.binding).Updates(map[string]any{"start_at": startMS, "end_at": endMS}).Error)
	stream, current := f.current(t)
	require.Equal(t, Denied, current.State)
	var selected im.AuthorizationSegment
	at := from.Add(time.Minute)
	require.NoError(t, f.db.Where("stream_uuid=? AND effective_from<=? AND (effective_until IS NULL OR effective_until>?)", stream.UUID.String(), at, at).Take(&selected).Error)
	require.Equal(t, Allowed, selected.State)
	require.Equal(t, from, *selected.GrantedAt)
	require.Equal(t, until, *selected.ValidUntil)
	var countBefore, countAfter int64
	require.NoError(t, f.db.Model(&im.AuthorizationSegment{}).Count(&countBefore).Error)
	require.NoError(t, f.db.Where("stream_uuid=? AND effective_from<=? AND (effective_until IS NULL OR effective_until>?)", stream.UUID.String(), at, at).Take(&selected).Error)
	require.NoError(t, f.db.Model(&im.AuthorizationSegment{}).Count(&countAfter).Error)
	require.Equal(t, countBefore, countAfter)
}

func TestIndirectTeamMembershipAndDimensionStatusMaintainEpochs(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.db.AutoMigrate(&im.Team{}))
	team := im.Team{TenantUUID: f.tenant, Key: "production", Name: "Production", Status: 1}
	require.NoError(t, f.db.Create(&team).Error)
	assignment := im.MemberAssignment{TenantUUID: f.tenant, MemberID: f.member.ID, DimType: im.DimTeam, DimID: team.ID}
	indirect := im.RoleBinding{TenantUUID: f.tenant, RoleID: f.role.ID, RoleUUID: f.role.UUID.String(), SubjectType: im.SubTeam, SubjectID: team.ID, DataScope: im.ScopeTenant}
	_, first := f.current(t)
	require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&assignment).Error; err != nil {
			return err
		}
		if err := tx.Create(&indirect).Error; err != nil {
			return err
		}
		return tx.Delete(&f.binding).Error
	}))
	_, inherited := f.current(t)
	require.Equal(t, first.EpochUUID, inherited.EpochUUID)
	require.NoError(t, f.db.Model(&team).Update("status", 2).Error)
	_, disabled := f.current(t)
	require.Equal(t, Denied, disabled.State)
	require.NoError(t, f.db.Model(&team).Update("status", 1).Error)
	_, enabled := f.current(t)
	require.Equal(t, Allowed, enabled.State)
	require.NotEqual(t, first.EpochUUID, enabled.EpochUUID)
	require.NoError(t, f.db.Delete(&assignment).Error)
	_, left := f.current(t)
	require.Equal(t, Denied, left.State)
}

func TestSubjectAndPermissionDisablingCloseTheContinuousEpoch(t *testing.T) {
	for _, subject := range []string{"user", "tenant", "plugin", "permission"} {
		t.Run(subject, func(t *testing.T) {
			f := newFixture(t)
			_, before := f.current(t)
			var source *gorm.DB
			column, off, on := "status", any(2), any(1)
			switch subject {
			case "user":
				source = f.db.Model(&im.User{}).Where("uuid=?", f.member.UserUUID)
			case "tenant":
				source = f.db.Model(&tm.Tenant{}).Where("uuid=?", f.tenant)
			case "plugin":
				source = f.db.Model(&sm.PluginInstanceConfig{}).Where("tenant_uuid=? AND plugin_id=?", f.tenant, f.plugin)
				column, off, on = "enabled", false, true
			case "permission":
				source = f.db.Model(&im.Permission{}).Where("id=?", f.permission.ID)
				off, on = im.PermissionStatusDeprecated, im.PermissionStatusActive
			}
			require.NoError(t, source.Session(&gorm.Session{}).Update(column, off).Error)
			_, denied := f.current(t)
			require.Equal(t, Denied, denied.State)
			require.NoError(t, source.Session(&gorm.Session{}).Update(column, on).Error)
			_, resumed := f.current(t)
			require.Equal(t, Allowed, resumed.State)
			require.NotEqual(t, before.EpochUUID, resumed.EpochUUID)
		})
	}
}
