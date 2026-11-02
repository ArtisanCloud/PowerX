package storagegov

import (
	"context"
	"encoding/json"
	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	tenants "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"path/filepath"
	"testing"
	"time"
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
	ctx := reqctx.WithIsRoot(reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true}), true)
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
	require.Error(t, err, "missing migration must block enable")
	require.NoError(t, db.AutoMigrate(&capmodel.InvocationTrace{}, &capmodel.CapabilityEventPublication{}, &tenants.Tenant{}))
	p, err = s.SaveDatabaseRetention(ctx, 0, p)
	require.NoError(t, err)
	require.True(t, p.Enabled)
	_, err = s.SaveDatabaseRetention(ctx, 0, p)
	require.ErrorIs(t, err, ErrPolicyConflict)
	preview, err := s.PreviewDatabaseRetention(ctx, p.Revision)
	require.NoError(t, err)
	require.Equal(t, "preview", preview.State)
	require.NotNil(t, preview.PolicySnapshot)
	require.Equal(t, p.SuccessDays, preview.PolicySnapshot.SuccessDays)
	key := uuid.NewString()
	_, err = coordinator.Document(ctx, "database-retention", func(raw []byte) ([]byte, error) {
		d, e := s.databaseDocument(raw)
		if e != nil {
			return nil, e
		}
		d.Runs = []DatabaseRetentionRun{{ID: key, Revision: p.Revision, State: "unconfirmed", At: time.Now()}}
		return json.Marshal(d)
	})
	require.NoError(t, err)
	run, err := s.RunDatabaseRetention(ctx, p.Revision, key)
	require.NoError(t, err)
	require.Equal(t, "unconfirmed", run.State)
	status, err = s.DatabaseRetention(ctx)
	require.NoError(t, err)
	require.Len(t, status.Runs, 1)
	require.Equal(t, 1, status.UnconfirmedRuns)
	_, err = coordinator.Document(ctx, "database-retention", func(raw []byte) ([]byte, error) {
		d, e := s.databaseDocument(raw)
		if e != nil {
			return nil, e
		}
		d.Runs = nil
		for i := 0; i < 100; i++ {
			d.Runs = append(d.Runs, DatabaseRetentionRun{ID: uuid.NewString(), State: "unconfirmed"})
		}
		return json.Marshal(d)
	})
	require.NoError(t, err)
	_, err = s.RunDatabaseRetention(ctx, p.Revision, uuid.NewString())
	require.ErrorIs(t, err, ErrStateUnavailable, "unconfirmed results must be bounded without dropping their evidence")
	status, err = s.DatabaseRetention(ctx)
	require.NoError(t, err)
	require.Equal(t, 100, status.UnconfirmedRuns)
	require.Len(t, status.Runs, 100)
	_, err = s.PreviewDatabaseRetention(ctx, p.Revision)
	require.NoError(t, err, "the evidence limit must not block read-only diagnosis")
	require.NoError(t, s.CheckDatabaseAdmission(ctx), "admission is a distinct opt-in")
	p.Enabled = false
	p.RejectNewWhenFull = true
	p, err = s.SaveDatabaseRetention(ctx, p.Revision, p)
	require.NoError(t, err)
	require.Error(t, s.CheckDatabaseAdmission(ctx), "unknown physical usage must fail closed even when deletion is paused")
}

func TestDatabaseRetentionObservationReturnsReadinessWithoutCreatingRuntimeState(t *testing.T) {
	s, err := NewService(Options{FilesystemPath: t.TempDir(), Collector: runtimeUnknownCollector{}, Identity: Identity{InstanceID: "dev"}})
	require.NoError(t, err)
	_, err = s.DatabaseRetention(context.Background())
	require.ErrorIs(t, err, ErrRootRequired)
	ctx := reqctx.WithIsRoot(reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true}), true)
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
	require.ErrorIs(t, err, ErrStateUnavailable)
}
