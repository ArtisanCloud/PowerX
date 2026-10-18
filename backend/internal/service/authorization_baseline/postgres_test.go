package authorization_baseline

import (
	"context"
	"os"
	"testing"
	"time"

	coredb "github.com/ArtisanCloud/PowerX/pkg/corex/db"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func postgresServiceFixture(t *testing.T) fixture {
	t.Helper()
	dsn := os.Getenv("POWERX_BASELINE_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POWERX_BASELINE_POSTGRES_DSN is not set")
	}
	old := core.PowerXSchema
	schema := "auth_service_" + uuid.NewString()[:8]
	core.PowerXSchema = schema
	t.Cleanup(func() { core.PowerXSchema = old })
	db, err := database.Connect(coredb.DatabaseConfig{Driver: "postgres", DSN: dsn, LogLevel: "silent", MaxOpenConns: 8})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	return seedServiceFixture(t, db, false)
}

func TestPostgresProductionConnectionFactoryAndServiceContract(t *testing.T) {
	f := postgresServiceFixture(t)
	f.initialize(t, "operations.order:read")
	result, err := f.s.ReadMemberBaselines(f.svc, f.query("operations.order:read"))
	require.NoError(t, err)
	require.True(t, *result.Items[0].Allowed)
	require.Nil(t, result.Items[0].GrantedAt)
	require.NotNil(t, result.Items[0].VisibleSince)
	require.NoError(t, f.db.Where("tenant_uuid=? AND subject_uuid=?", f.tenant, f.member.UUID.String()).Delete(&im.RoleBinding{}).Error)
	result, err = f.s.ReadMemberBaselines(f.svc, f.query("operations.order:read"))
	require.NoError(t, err)
	require.False(t, *result.Items[0].Allowed)
}

func TestPostgresInitializationAndRoleMembershipUseTheSameBoundary(t *testing.T) {
	f := postgresServiceFixture(t)
	user := im.User{Email: uuid.NewString() + "@example.invalid", Status: 1}
	require.NoError(t, f.db.Create(&user).Error)
	member := im.Member{TenantUUID: f.tenant, UserID: user.ID, UserUUID: user.UUID.String(), Username: "joining", Status: 1}
	require.NoError(t, f.db.Create(&member).Error)
	var role im.Role
	require.NoError(t, f.db.Where("tenant_uuid=?", f.tenant).Take(&role).Error)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 2)
	require.NoError(t, f.db.Callback().Create().Before("gorm:create").Register("test:hold_initialization", func(tx *gorm.DB) {
		if tx.Statement.Table == core.TableIAMAuthorizationTracking {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				tx.AddError(ctx.Err())
			}
		}
	}))
	go func() {
		_, err := f.s.Initialize(ctx, InitializationInput{TenantUUID: f.tenant, PluginID: f.plugin, PermissionCodes: []string{"operations.order:read"}, Apply: true, GuardUUID: uuid.NewString(), Operator: "acceptance"})
		done <- err
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	go func() {
		done <- f.db.WithContext(ctx).Create(&im.RoleBinding{TenantUUID: f.tenant, RoleID: role.ID, RoleUUID: role.UUID.String(), SubjectType: im.SubMember, SubjectID: member.ID, SubjectUUID: member.UUID.String()}).Error
	}()
	close(release)
	require.NoError(t, <-done)
	require.NoError(t, <-done)
	f.member = member
	result, err := f.s.ReadMemberBaselines(f.svc, f.query("operations.order:read"))
	require.NoError(t, err)
	require.True(t, *result.Items[0].Allowed)
	require.Equal(t, "native", result.Items[0].BaselineKind)
	require.NotNil(t, result.Items[0].GrantedAt)
}
