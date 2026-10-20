package authorizationbaseline

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	sm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func postgresFixture(t *testing.T) fixture {
	t.Helper()
	dsn := os.Getenv("POWERX_BASELINE_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POWERX_BASELINE_POSTGRES_DSN is not set")
	}
	old := model.PowerXSchema
	model.PowerXSchema = "auth_baseline_" + uuid.New().String()[:8]
	schema := model.PowerXSchema
	t.Cleanup(func() { model.PowerXSchema = old })
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(8)
	require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&im.AuthorizationTracking{}, &im.AuthorizationStream{}, &im.AuthorizationSegment{}, &im.User{}, &im.Member{}, &im.Role{}, &im.Permission{}, &im.RolePermission{}, &im.RoleBinding{}, &im.MemberAssignment{}, &im.Department{}, &im.Group{}, &im.Team{}, &im.Position{}, &tm.Tenant{}, &sm.PluginInstanceConfig{}))
	return seedFixture(t, db)
}

func TestPostgresConcurrentRolePermissionAndNewRoleMember(t *testing.T) {
	for _, first := range []string{"permission", "member"} {
		t.Run(first, func(t *testing.T) {
			f := postgresFixture(t)
			require.NoError(t, f.db.Where("role_uuid=?", f.role.UUID.String()).Delete(&im.RolePermission{}).Error)
			user := im.User{Email: uuid.NewString() + "@example.invalid", Status: 1}
			require.NoError(t, f.db.Create(&user).Error)
			member := im.Member{TenantUUID: f.tenant, UserID: user.ID, UserUUID: user.UUID.String(), Username: "second", Status: 1}
			require.NoError(t, f.db.Create(&member).Error)
			addPermission := func(tx *gorm.DB) error {
				return tx.Create(&im.RolePermission{RoleID: f.role.ID, RoleUUID: f.role.UUID, PermissionID: f.permission.ID, PermissionUUID: f.permission.PermissionUUID}).Error
			}
			addMember := func(tx *gorm.DB) error {
				return tx.Create(&im.RoleBinding{TenantUUID: f.tenant, RoleID: f.role.ID, RoleUUID: f.role.UUID.String(), SubjectType: im.SubMember, SubjectID: member.ID, SubjectUUID: member.UUID.String(), DataScope: im.ScopeTenant}).Error
			}
			a, b := addPermission, addMember
			if first == "member" {
				a, b = b, a
			}
			entered, release := make(chan struct{}), make(chan struct{})
			done := make(chan error, 2)
			go func() {
				done <- f.db.Transaction(func(tx *gorm.DB) error {
					if err := Lock(tx); err != nil {
						return err
					}
					close(entered)
					<-release
					return a(tx)
				})
			}()
			<-entered
			go func() { done <- f.db.Transaction(func(tx *gorm.DB) error { return b(tx) }) }()
			close(release)
			require.NoError(t, <-done)
			require.NoError(t, <-done)
			f.member = member
			_, segment := f.current(t)
			require.Equal(t, Allowed, segment.State)
			require.NotNil(t, segment.GrantedAt)
		})
	}
}

func TestPostgresOneReadSnapshotCannotMixCostAndReadRevisions(t *testing.T) {
	f := postgresFixture(t)
	first, _ := f.current(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, f.db.WithContext(ctx).Transaction(func(reader *gorm.DB) error {
		var before im.AuthorizationStream
		if err := reader.Where("uuid=?", first.UUID).Take(&before).Error; err != nil {
			return err
		}
		done := make(chan error, 1)
		go func() { done <- f.db.Model(&f.member).Update("status", 2).Error }()
		require.NoError(t, <-done)
		var after im.AuthorizationStream
		if err := reader.Where("uuid=?", first.UUID).Take(&after).Error; err != nil {
			return err
		}
		require.Equal(t, before.FactRevision, after.FactRevision)
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}))
	_, denied := f.current(t)
	require.Equal(t, Denied, denied.State)
}

func TestPostgresExplicitConnectionKeepsTheTransactionCoordinator(t *testing.T) {
	f := postgresFixture(t)
	require.NoError(t, f.db.Connection(func(conn *gorm.DB) error {
		BindConnection(conn)
		return conn.Model(&f.member).Update("status", 2).Error
	}))
	_, disabled := f.current(t)
	require.Equal(t, Denied, disabled.State)
}
