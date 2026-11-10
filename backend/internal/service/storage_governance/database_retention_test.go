package storagegov

import (
	"context"
	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	tenants "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"path/filepath"
	"testing"
)

func TestDatabasePolicyIsIndependentDefaultOffAndUnknownRunsAreNotReplayed(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "audit.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	old := core.PowerXSchema
	core.PowerXSchema = "main"
	defer func() { core.PowerXSchema = old }()
	coordinator, err := budget.New(budget.Config{Directory: filepath.Join(t.TempDir(), "runtime"), FilesystemPath: t.TempDir(), HostID: "host", FilesystemID: "fs", InstanceID: "dev", MinFreeBytes: 1 << 20, CriticalFreeBytes: 1})
	require.NoError(t, err)
	s, err := NewService(Options{FilesystemPath: t.TempDir(), Collector: runtimeUnknownCollector{}, DB: db, Budget: coordinator, Identity: Identity{HostID: "host", FilesystemID: "fs", InstanceID: "dev"}, DatabaseRetentionAllowed: true})
	require.NoError(t, err)
	ctx := reqctx.WithIsRoot(reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access"}), true)
	_, err = s.DatabaseRetention(context.Background())
	require.ErrorIs(t, err, ErrRootRequired)
	status, err := s.DatabaseRetention(ctx)
	require.NoError(t, err)
	require.True(t, status.StateConfigured)
	require.True(t, status.EnforceAllowed)
	require.False(t, status.MigrationReady)
	require.Equal(t, "database_retention_migration_required", status.MigrationProblem)
	require.False(t, status.Policy.Enabled)
	require.False(t, status.Ready)
	p := status.Policy
	p.Enabled = true
	_, err = s.SaveDatabaseRetention(ctx, 0, p)
	require.Equal(t, "STORAGE_CONTROL_PRECONDITION_REQUIRED", dto.CodeOf(err))
	require.NoError(t, db.AutoMigrate(&capmodel.InvocationTrace{}, &capmodel.CapabilityEventPublication{}, &tenants.Tenant{}))
	_, err = s.SaveDatabaseRetention(ctx, 0, p)
	require.Equal(t, "STORAGE_CONTROL_PRECONDITION_REQUIRED", dto.CodeOf(err), "schema alone never enables legacy file writes")
	_, err = s.RunDatabaseRetention(ctx, 0, uuid.NewString())
	require.Equal(t, "DATABASE_RETENTION_PREVIEW_REQUIRED", dto.CodeOf(err))
	require.Error(t, s.CheckDatabaseAdmission(ctx), "configured runtime without authority cannot bypass admission")

}

func TestDatabaseRetentionObservationReturnsReadinessWithoutCreatingRuntimeState(t *testing.T) {
	s, err := NewService(Options{FilesystemPath: t.TempDir(), Collector: runtimeUnknownCollector{}, Identity: Identity{InstanceID: "dev"}})
	require.NoError(t, err)
	_, err = s.DatabaseRetention(context.Background())
	require.ErrorIs(t, err, ErrRootRequired)
	ctx := reqctx.WithIsRoot(reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access"}), true)
	status, err := s.DatabaseRetention(ctx)
	require.NoError(t, err)
	require.False(t, status.StateConfigured)
	require.False(t, status.EnforceAllowed)
	require.False(t, status.Ready)
	require.False(t, status.Policy.Enabled)
	require.False(t, status.Policy.RejectNewWhenFull)
	require.NotNil(t, status.Runs)
	require.Nil(t, status.RelationBytes)
	require.Equal(t, "database_retention_runtime_not_configured", status.Problem)
	require.Equal(t, "database_unavailable", status.MigrationProblem)
	_, err = s.SaveDatabaseRetention(ctx, 0, status.Policy)
	require.Equal(t, "STORAGE_CONTROL_PRECONDITION_REQUIRED", dto.CodeOf(err))
}
