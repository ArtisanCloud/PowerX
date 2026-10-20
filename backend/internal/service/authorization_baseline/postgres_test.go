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
	baseline "github.com/ArtisanCloud/PowerX/pkg/corex/iam/authorizationbaseline"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
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
	result, err := f.s.ReadMemberBaselines(f.svc, f.query("operations.order:read"))
	require.NoError(t, err)
	require.True(t, *result.Items[0].Allowed)
	// The production factory tracks the initial deny before the role grant commits.
	require.Equal(t, "native", result.Items[0].BaselineKind)
	require.NotNil(t, result.Items[0].GrantedAt)
	require.NotNil(t, result.Items[0].VisibleSince)
	require.NoError(t, f.db.Where("tenant_uuid=? AND subject_uuid=?", f.tenant, f.member.UUID.String()).Delete(&im.RoleBinding{}).Error)
	result, err = f.s.ReadMemberBaselines(f.svc, f.query("operations.order:read"))
	require.NoError(t, err)
	require.False(t, *result.Items[0].Allowed)
}

func TestPostgresInitializationAndRoleMembershipUseTheSameBoundary(t *testing.T) {
	for _, first := range []string{"initialization", "membership"} {
		t.Run(first, func(t *testing.T) {
			f := postgresServiceFixture(t)
			user := im.User{Email: uuid.NewString() + "@example.invalid", Status: 1}
			require.NoError(t, f.db.Create(&user).Error)
			member := im.Member{TenantUUID: f.tenant, UserID: user.ID, UserUUID: user.UUID.String(), Username: "joining", Status: 1}
			require.NoError(t, f.db.Create(&member).Error)
			var role im.Role
			require.NoError(t, f.db.Where("tenant_uuid=?", f.tenant).Take(&role).Error)
			permission := im.Permission{Module: "operations", Resource: "audit", Action: "read", Source: "plugin:" + f.plugin, Meta: datatypes.JSON(`{"data_scope":"TENANT"}`)}
			require.NoError(t, f.db.Create(&permission).Error)
			require.NoError(t, f.db.Create(&im.RolePermission{RoleID: role.ID, RoleUUID: role.UUID, PermissionID: permission.ID, PermissionUUID: permission.PermissionUUID}).Error)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			entered, release := make(chan struct{}), make(chan struct{})
			done := make(chan error, 2)
			initialize := func(tx *gorm.DB) error {
				return tx.Model(&permission).Update("meta", datatypes.JSON(`{"authorization_baseline":true,"data_scope":"TENANT"}`)).Error
			}
			join := func(tx *gorm.DB) error {
				return tx.Create(&im.RoleBinding{TenantUUID: f.tenant, RoleID: role.ID, RoleUUID: role.UUID.String(), SubjectType: im.SubMember, SubjectID: member.ID, SubjectUUID: member.UUID.String(), DataScope: im.ScopeTenant}).Error
			}
			a, b := initialize, join
			if first == "membership" {
				a, b = b, a
			}
			go func() {
				done <- f.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
					if err := baseline.Lock(tx); err != nil {
						return err
					}
					if err := a(tx); err != nil {
						return err
					}
					close(entered)
					select {
					case <-release:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				})
			}()
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			go func() {
				done <- f.db.WithContext(ctx).Transaction(b)
			}()
			close(release)
			require.NoError(t, <-done)
			require.NoError(t, <-done)
			f.member = member
			result, err := f.s.ReadMemberBaselines(f.svc, f.query("operations.audit:read"))
			require.NoError(t, err)
			require.True(t, *result.Items[0].Allowed)
			if first == "initialization" {
				require.Equal(t, "native", result.Items[0].BaselineKind)
				require.NotNil(t, result.Items[0].GrantedAt)
			} else {
				require.Equal(t, "initialized", result.Items[0].BaselineKind)
				require.Nil(t, result.Items[0].GrantedAt)
			}
		})
	}
}
