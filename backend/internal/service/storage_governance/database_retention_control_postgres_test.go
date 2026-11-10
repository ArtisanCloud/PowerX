package storagegov

import (
	"context"
	"encoding/json"
	"errors"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	capm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	tenants "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func retentionNative(t *testing.T) (*Service, func(), *gorm.DB) {
	s, c, q, db := monitoringNative(t)
	s.opts.DatabaseRetentionAllowed = true
	s.opts.LifecycleWorkerEnabled = true
	require.NoError(t, db.AutoMigrate(&capm.InvocationTrace{}, &capm.CapabilityEventPublication{}, &tenants.Tenant{}))
	return s, func() { applyMonitoring(t, c, q) }, db
}
func adoptRetentionNative(t *testing.T, s *Service, apply func()) DatabaseRetentionPolicy {
	t.Helper()
	status, e := s.DatabaseRetention(controlRoot())
	require.NoError(t, e)
	require.Equal(t, "legacy", status.Configuration.Authority)
	_, e = s.AdoptDatabaseRetention(controlRoot(), DatabaseRetentionAdoption{uuid.NewString(), status.Configuration.LegacyDigest, true})
	require.NoError(t, e)
	apply()
	status, e = s.DatabaseRetention(controlRoot())
	require.NoError(t, e)
	require.True(t, status.Ready)
	require.False(t, status.Policy.Enabled)
	return status.Policy
}
func enableRetentionNative(t *testing.T, s *Service, apply func(), rev int64) DatabaseRetentionPolicy {
	t.Helper()
	yes := true
	_, e := s.ChangeDatabaseRetention(controlRoot(), DatabaseRetentionChange{RequestID: uuid.NewString(), ExpectedRevision: rev, Action: "status", Enabled: &yes, Confirm: true})
	require.NoError(t, e)
	apply()
	v, e := s.DatabaseRetention(controlRoot())
	require.NoError(t, e)
	return v.Policy
}
func seedRetentionNative(t *testing.T, db *gorm.DB) string {
	t.Helper()
	tenant := &tenants.Tenant{Key: uuid.NewString(), Name: "owned retention"}
	require.NoError(t, db.Create(tenant).Error)
	old := time.Now().UTC().AddDate(0, 0, -120)
	recent := time.Now().UTC()
	for _, tr := range []capm.InvocationTrace{{TenantUUID: tenant.UUID.String(), TraceID: uuid.NewString(), Status: "completed", RecordClass: capm.InvocationAuditClass, CompletedAt: &old}, {TenantUUID: tenant.UUID.String(), TraceID: uuid.NewString(), Status: "completed", RecordClass: capm.InvocationAuditClass, CompletedAt: &recent}, {TenantUUID: tenant.UUID.String(), TraceID: uuid.NewString(), Status: "completed", RecordClass: "legacy_unclassified", CompletedAt: &old}, {TenantUUID: uuid.NewString(), TraceID: uuid.NewString(), Status: "completed", RecordClass: capm.InvocationAuditClass, CompletedAt: &old}} {
		require.NoError(t, db.Create(&tr).Error)
	}
	require.NoError(t, db.Create(&capm.CapabilityEventPublication{TenantUUID: tenant.UUID.String(), TraceID: uuid.NewString(), Status: "pending", RecordClass: "legacy_unclassified", Topic: "business.order.created", Payload: datatypes.JSON(`{}`)}).Error)
	return tenant.UUID.String()
}
func retentionRows(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Unscoped().Model(&capm.InvocationTrace{}).Count(&n).Error)
	return n
}
func TestDatabaseRetentionControlNativeAdoptionCASReceiptsAndPausedSave(t *testing.T) {
	s, apply, db := retentionNative(t)
	ctx := controlRoot()
	legacy, e := s.opts.Budget.Document(ctx, "database-retention", nil)
	require.NoError(t, e)
	p := adoptRetentionNative(t, s, apply)
	raw, e := s.opts.Budget.Document(ctx, "database-retention", nil)
	require.NoError(t, e)
	require.Equal(t, legacy, raw)
	p = enableRetentionNative(t, s, apply, p.Revision)
	settings := retentionSettings(p)
	settings.SuccessDays = 7
	in := DatabaseRetentionChange{RequestID: uuid.NewString(), ExpectedRevision: p.Revision, Action: "save", Settings: &settings, Confirm: true}
	saved, e := s.ChangeDatabaseRetention(ctx, in)
	require.NoError(t, e)
	again, e := s.ChangeDatabaseRetention(ctx, in)
	require.NoError(t, e)
	require.Equal(t, saved.OperationRevision, again.OperationRevision)
	settings.SuccessDays = 8
	_, e = s.ChangeDatabaseRetention(ctx, in)
	require.Equal(t, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT", dto.CodeOf(e))
	settings.SuccessDays = 7
	apply()
	v, e := s.DatabaseRetention(ctx)
	require.NoError(t, e)
	require.False(t, v.Policy.Enabled)
	require.Equal(t, 7, v.Policy.SuccessDays)
	gate := make(chan struct{})
	results := make(chan error, 6)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-gate
			p := settings
			p.SuccessDays = 10 + i
			_, e := s.ChangeDatabaseRetention(ctx, DatabaseRetentionChange{RequestID: uuid.NewString(), ExpectedRevision: v.Policy.Revision, Action: "save", Settings: &p, Confirm: true})
			results <- e
		}(i)
	}
	close(gate)
	wg.Wait()
	close(results)
	wins := 0
	for e := range results {
		if e == nil {
			wins++
		} else {
			require.Equal(t, "STORAGE_CONTROL_REVISION_CONFLICT", dto.CodeOf(e))
		}
	}
	require.Equal(t, 1, wins)
	var n int64
	require.NoError(t, db.Model(&m.ControlRevision{}).Where("domain = ?", ControlDatabaseRetention).Count(&n).Error)
	require.EqualValues(t, 4, n)
	_, e = s.SaveDatabaseRetention(ctx, v.Policy.Revision, v.Policy)
	require.Equal(t, "STORAGE_CONTROL_PRECONDITION_REQUIRED", dto.CodeOf(e))
	_, e = s.RunDatabaseRetention(ctx, v.Policy.Revision, uuid.NewString())
	require.Equal(t, "DATABASE_RETENTION_PREVIEW_REQUIRED", dto.CodeOf(e))
	_, e = s.DatabaseRetentionRequest(context.Background(), in.RequestID)
	require.ErrorIs(t, e, ErrRootRequired)
}
func TestDatabaseRetentionControlNativePreviewAtomicDeleteAndReceipt(t *testing.T) {
	s, apply, db := retentionNative(t)
	ctx := controlRoot()
	p := adoptRetentionNative(t, s, apply)
	p = enableRetentionNative(t, s, apply, p.Revision)
	seedRetentionNative(t, db)
	pool, e := db.DB()
	require.NoError(t, e)
	pool.SetMaxOpenConns(1)
	preview, e := s.PreviewDatabaseRetention(ctx, p.Revision)
	require.NoError(t, e)
	require.EqualValues(t, 1, preview.Counts.Traces)
	require.NotEmpty(t, preview.ID)
	require.EqualValues(t, 4, retentionRows(t, db))
	key := uuid.NewString()
	run, e := s.RunConfirmedDatabaseRetention(ctx, p.Revision, key, preview.ID)
	require.NoError(t, e)
	require.Equal(t, "completed", run.State)
	require.EqualValues(t, 1, run.Counts.Traces)
	require.EqualValues(t, 3, retentionRows(t, db))
	receipt, e := s.DatabaseRetentionRunRequest(ctx, key)
	require.NoError(t, e)
	require.Equal(t, run.Counts, receipt.Counts)
	_, e = s.RunConfirmedDatabaseRetention(ctx, p.Revision, uuid.NewString(), preview.ID)
	require.Equal(t, "DATABASE_RETENTION_PREVIEW_EXPIRED", dto.CodeOf(e))
	again, e := s.RunConfirmedDatabaseRetention(ctx, p.Revision, key, preview.ID)
	require.NoError(t, e)
	require.Equal(t, run.Counts, again.Counts)
	yes := false
	_, e = s.ChangeDatabaseRetention(ctx, DatabaseRetentionChange{RequestID: uuid.NewString(), ExpectedRevision: p.Revision, Action: "status", Enabled: &yes, Confirm: true})
	require.NoError(t, e)
	again, e = s.RunConfirmedDatabaseRetention(ctx, p.Revision, key, preview.ID)
	require.NoError(t, e)
	require.Equal(t, run.Counts, again.Counts)
	require.EqualValues(t, 3, retentionRows(t, db))
	_, e = s.PreviewDatabaseRetention(ctx, p.Revision)
	require.Equal(t, "DATABASE_RETENTION_PUBLICATION_PENDING", dto.CodeOf(e))
	apply()
}
func TestDatabaseRetentionControlNativeRollbackAndExpiredPreview(t *testing.T) {
	s, apply, db := retentionNative(t)
	ctx := controlRoot()
	p := adoptRetentionNative(t, s, apply)
	p = enableRetentionNative(t, s, apply, p.Revision)
	seedRetentionNative(t, db)
	preview, e := s.PreviewDatabaseRetention(ctx, p.Revision)
	require.NoError(t, e)
	key := uuid.NewString()
	name := "owned_retention_receipt_failure"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if r, ok := tx.Statement.Dest.(*m.CleanupRun); ok && r.RunID == key {
			tx.AddError(errors.New("owned receipt persistence failed"))
		}
	}))
	_, e = s.RunConfirmedDatabaseRetention(ctx, p.Revision, key, preview.ID)
	require.Error(t, e)
	require.EqualValues(t, 4, retentionRows(t, db))
	_, e = s.DatabaseRetentionRunRequest(ctx, key)
	require.Equal(t, "DATABASE_RETENTION_RUN_NOT_FOUND", dto.CodeOf(e))
	require.NoError(t, db.Callback().Create().Remove(name))
	row, e := s.retentionLedger().Get(ctx, db, preview.ID)
	require.NoError(t, e)
	require.Equal(t, "preview", row.State)
	s.opts.Now = func() time.Time { return preview.At.Add(6 * time.Minute) }
	_, e = s.RunConfirmedDatabaseRetention(ctx, p.Revision, uuid.NewString(), preview.ID)
	require.Equal(t, "DATABASE_RETENTION_PREVIEW_EXPIRED", dto.CodeOf(e))
	require.EqualValues(t, 4, retentionRows(t, db))
}
func TestDatabaseRetentionControlNativeTamperRollbackAndLegacyUnknown(t *testing.T) {
	s, apply, db := retentionNative(t)
	ctx := controlRoot()
	p := adoptRetentionNative(t, s, apply)
	p = enableRetentionNative(t, s, apply, p.Revision)
	seedRetentionNative(t, db)
	file := filepath.Join(s.opts.Budget.ChangeDirectory(), "database-retention-control.json")
	raw, e := os.ReadFile(file)
	require.NoError(t, e)
	var projection snapshotPolicyProjection
	require.NoError(t, json.Unmarshal(raw, &projection))
	projection.Digest = "tampered"
	bad, _ := json.Marshal(projection)
	require.NoError(t, os.WriteFile(file, bad, 0600))
	_, e = s.PreviewDatabaseRetention(ctx, p.Revision)
	require.Error(t, e)
	require.Error(t, s.CheckDatabaseAdmission(ctx))
	require.EqualValues(t, 4, retentionRows(t, db))
	require.NoError(t, os.WriteFile(file, raw, 0600))
	require.NoError(t, db.Where("tenant_id = ? AND scope_id = ? AND domain = ?", 0, s.opts.Control.opts.Scope.ID(), ControlDatabaseRetention).Delete(&m.ControlConfiguration{}).Error)
	require.NoError(t, os.Remove(file))
	_, e = s.PreviewDatabaseRetention(ctx, p.Revision)
	require.Error(t, e)
	v, e := s.DatabaseRetention(ctx)
	require.NoError(t, e)
	require.Equal(t, "DATABASE_RETENTION_RESTORE_CONFIRMATION_REQUIRED", v.Configuration.Problem)
}
func TestDatabaseRetentionControlNativePendingCapacityProtectionAndLegacyUnconfirmed(t *testing.T) {
	s, apply, db := retentionNative(t)
	ctx := controlRoot()
	legacy := databaseRetentionDocument{Policy: DatabaseRetentionPolicy{OwnerInstance: s.opts.Identity.InstanceID, SuccessDays: 30, FailureDays: 90, PendingHours: 24, QuarantineDays: 30, BatchLimit: 200, MaxBytes: 8 << 30}, Runs: []DatabaseRetentionRun{{ID: uuid.NewString(), State: "unconfirmed"}}}
	_, e := s.opts.Budget.Document(ctx, "database-retention", func([]byte) ([]byte, error) { return json.Marshal(legacy) })
	require.NoError(t, e)
	v, e := s.DatabaseRetention(ctx)
	require.NoError(t, e)
	_, e = s.AdoptDatabaseRetention(ctx, DatabaseRetentionAdoption{uuid.NewString(), v.Configuration.LegacyDigest, true})
	require.Equal(t, "DATABASE_RETENTION_LEGACY_RESULT_UNCONFIRMED", dto.CodeOf(e))
	legacy.Runs = []DatabaseRetentionRun{}
	_, e = s.opts.Budget.Document(ctx, "database-retention", func([]byte) ([]byte, error) { return json.Marshal(legacy) })
	require.NoError(t, e)
	p := adoptRetentionNative(t, s, apply)
	settings := retentionSettings(p)
	settings.RejectNewWhenFull = true
	settings.MaxBytes = 64 << 20
	_, e = s.ChangeDatabaseRetention(ctx, DatabaseRetentionChange{RequestID: uuid.NewString(), ExpectedRevision: p.Revision, Action: "save", Settings: &settings, Confirm: true})
	require.NoError(t, e)
	_, c, e := s.loadRetentionControl(ctx)
	require.NoError(t, e)
	require.True(t, c.EffectiveCapacity.RejectNewWhenFull)
	require.EqualValues(t, 64<<20, c.EffectiveCapacity.MaxBytes)
	apply()
	settings.RejectNewWhenFull = false
	settings.MaxBytes = 8 << 30
	_, e = s.ChangeDatabaseRetention(ctx, DatabaseRetentionChange{RequestID: uuid.NewString(), ExpectedRevision: 2, Action: "save", Settings: &settings, Confirm: true})
	require.NoError(t, e)
	_, c, e = s.loadRetentionControl(ctx)
	require.NoError(t, e)
	require.True(t, c.EffectiveCapacity.RejectNewWhenFull)
	require.EqualValues(t, 64<<20, c.EffectiveCapacity.MaxBytes)
	apply()
	_, c, e = s.loadRetentionControl(ctx)
	require.NoError(t, e)
	require.False(t, c.EffectiveCapacity.RejectNewWhenFull)
	_ = db
}

func TestDatabaseRetentionControlNativeExternalReferenceAndMissingLedgerIndex(t *testing.T) {
	s, apply, db := retentionNative(t)
	ctx := controlRoot()
	p := adoptRetentionNative(t, s, apply)
	p = enableRetentionNative(t, s, apply, p.Revision)
	seedRetentionNative(t, db)
	preview, e := s.PreviewDatabaseRetention(ctx, p.Revision)
	require.NoError(t, e)
	// 外部级联引用即使在预览后新增，也必须保护日志行及引用它的业务行。
	table := capm.InvocationTrace{}.TableName()
	child := table + "_owned_reference"
	require.NoError(t, db.Exec("CREATE TABLE "+child+" (id BIGSERIAL PRIMARY KEY, trace_id BIGINT REFERENCES "+table+"(id) ON DELETE CASCADE)").Error)
	require.NoError(t, db.Exec("INSERT INTO "+child+"(trace_id) SELECT id FROM "+table+" ORDER BY id LIMIT 1").Error)
	_, e = s.RunConfirmedDatabaseRetention(ctx, p.Revision, uuid.NewString(), preview.ID)
	require.ErrorContains(t, e, "database_external_references_require_review")
	require.EqualValues(t, 4, retentionRows(t, db))
	var n int64
	require.NoError(t, db.Table(child).Count(&n).Error)
	require.EqualValues(t, 1, n)
	require.NoError(t, db.Exec("DROP TABLE "+child).Error)
	require.NoError(t, db.Exec("DROP INDEX "+coremodel.PowerXSchema+".uk_storage_cleanup_run").Error)
	_, e = s.PreviewDatabaseRetention(ctx, p.Revision)
	require.Equal(t, "DATABASE_RETENTION_LEDGER_UNAVAILABLE", dto.CodeOf(e))
	require.EqualValues(t, 4, retentionRows(t, db))
}
