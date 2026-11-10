package storagegov

import (
	"context"
	"encoding/json"
	"errors"
	capm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	tenants "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func systemRetentionNative(t *testing.T) (*Service, func(), *gorm.DB) {
	s, c, q, db := monitoringNative(t)
	s.opts.DatabaseRetentionAllowed = true
	s.opts.LifecycleWorkerEnabled = true
	require.NoError(t, db.AutoMigrate(&capm.HealthProbeResult{}, &capm.DiscoveryCacheEntry{}, &tenants.Tenant{}))
	return s, func() { applyMonitoring(t, c, q) }, db
}
func adoptSystemNative(t *testing.T, s *Service, apply func(), id string) SystemTableRetentionStatus {
	t.Helper()
	status, e := s.SystemTableRetention(controlRoot(), id)
	require.NoError(t, e)
	require.Equal(t, "legacy", status.Configuration.Authority)
	_, e = s.AdoptSystemTableRetention(controlRoot(), id, SystemTableRetentionAdoption{uuid.NewString(), status.Configuration.LegacyDigest, true})
	require.NoError(t, e)
	apply()
	status, e = s.SystemTableRetention(controlRoot(), id)
	require.NoError(t, e)
	require.True(t, status.Ready, status.Problem)
	require.False(t, status.Policy.Enabled)
	return status
}
func enableSystemNative(t *testing.T, s *Service, apply func(), id string, rev int64) SystemTableRetentionStatus {
	t.Helper()
	yes := true
	_, e := s.ChangeSystemTableRetention(controlRoot(), id, SystemTableRetentionChange{RequestID: uuid.NewString(), ExpectedRevision: rev, Action: "status", Enabled: &yes, Confirm: true})
	require.NoError(t, e)
	apply()
	v, e := s.SystemTableRetention(controlRoot(), id)
	require.NoError(t, e)
	return v
}
func seedSystemNative(t *testing.T, db *gorm.DB) {
	t.Helper()
	now := time.Now().UTC()
	old := now.AddDate(0, 0, -45)
	tenant := &tenants.Tenant{Key: uuid.NewString(), Name: "owned system retention"}
	require.NoError(t, db.Create(tenant).Error)
	rows := []capm.HealthProbeResult{{AdapterID: "primary", ProbeWindowStart: old, Status: "healthy", NextRetryAt: old, CreatedAt: old, UpdatedAt: old}, {AdapterID: "primary", ProbeWindowStart: now, Status: "healthy", NextRetryAt: now, CreatedAt: now, UpdatedAt: now}, {AdapterID: "primary", ProbeWindowStart: old.Add(time.Hour), Status: "pending", NextRetryAt: old, CreatedAt: old, UpdatedAt: old}, {AdapterID: "primary", ProbeWindowStart: old.Add(2 * time.Hour), Status: "unhealthy", NextRetryAt: now.Add(time.Hour), CreatedAt: old, UpdatedAt: old}, {AdapterID: "last-only", ProbeWindowStart: old, Status: "healthy", NextRetryAt: old, CreatedAt: old, UpdatedAt: old}}
	require.NoError(t, db.Create(&rows).Error)
	for _, entry := range []capm.DiscoveryCacheEntry{{TenantUUID: tenant.UUID.String(), CapabilityID: "retired", ClientID: "owned", ExpiresAt: old, IssuedAt: old, UpdatedAt: old, DeletedAt: gorm.DeletedAt{Time: old, Valid: true}}, {TenantUUID: tenant.UUID.String(), CapabilityID: "active-expired", ClientID: "owned", ExpiresAt: old, IssuedAt: old, UpdatedAt: old}, {TenantUUID: tenant.UUID.String(), CapabilityID: "recent-retired", ClientID: "owned", ExpiresAt: old, IssuedAt: old, UpdatedAt: now, DeletedAt: gorm.DeletedAt{Time: now, Valid: true}}, {TenantUUID: uuid.NewString(), CapabilityID: "unknown-tenant", ClientID: "owned", ExpiresAt: old, IssuedAt: old, UpdatedAt: old, DeletedAt: gorm.DeletedAt{Time: old, Valid: true}}} {
		require.NoError(t, db.Create(&entry).Error)
	}
}
func systemRows(t *testing.T, db *gorm.DB) [2]int64 {
	t.Helper()
	var n [2]int64
	require.NoError(t, db.Unscoped().Model(&capm.HealthProbeResult{}).Count(&n[0]).Error)
	require.NoError(t, db.Unscoped().Model(&capm.DiscoveryCacheEntry{}).Count(&n[1]).Error)
	return n
}
func TestSystemRetentionControlNativeIndependentCASAndPausedSave(t *testing.T) {
	s, apply, _ := systemRetentionNative(t)
	ctx := controlRoot()
	before, e := s.opts.Budget.Document(ctx, "system-table-retention", nil)
	require.NoError(t, e)
	probe := adoptSystemNative(t, s, apply, repo.ProbeRetentionID)
	cache := adoptSystemNative(t, s, apply, repo.DiscoveryRetentionID)
	require.EqualValues(t, 1, probe.Revision)
	require.EqualValues(t, 1, cache.Revision)
	probe = enableSystemNative(t, s, apply, repo.ProbeRetentionID, 1)
	settings := SystemTableRetentionSettings{14, 100}
	in := SystemTableRetentionChange{RequestID: uuid.NewString(), ExpectedRevision: probe.Revision, Action: "save", Settings: &settings, Confirm: true}
	a, e := s.ChangeSystemTableRetention(ctx, repo.ProbeRetentionID, in)
	require.NoError(t, e)
	replay, e := s.ChangeSystemTableRetention(ctx, repo.ProbeRetentionID, in)
	require.NoError(t, e)
	require.Equal(t, a.OperationRevision, replay.OperationRevision)
	settings.RetentionDays = 15
	_, e = s.ChangeSystemTableRetention(ctx, repo.ProbeRetentionID, in)
	require.Equal(t, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT", dto.CodeOf(e))
	apply()
	v, e := s.SystemTableRetention(ctx, repo.ProbeRetentionID)
	require.NoError(t, e)
	require.False(t, v.Policy.Enabled)
	cache, e = s.SystemTableRetention(ctx, repo.DiscoveryRetentionID)
	require.NoError(t, e)
	require.EqualValues(t, 1, cache.Revision)
	require.True(t, cache.Configuration.ExecutionReady)
	gate := make(chan struct{})
	out := make(chan error, 6)
	var wg sync.WaitGroup
	for n := 0; n < 6; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			<-gate
			p := SystemTableRetentionSettings{RetentionDays: 20 + n, BatchLimit: 100}
			_, e := s.ChangeSystemTableRetention(ctx, repo.ProbeRetentionID, SystemTableRetentionChange{RequestID: uuid.NewString(), ExpectedRevision: v.Revision, Action: "save", Settings: &p, Confirm: true})
			out <- e
		}(n)
	}
	close(gate)
	wg.Wait()
	close(out)
	wins := 0
	for e := range out {
		if e == nil {
			wins++
		} else {
			require.Equal(t, "STORAGE_CONTROL_REVISION_CONFLICT", dto.CodeOf(e))
		}
	}
	require.Equal(t, 1, wins)
	after, e := s.opts.Budget.Document(ctx, "system-table-retention", nil)
	require.NoError(t, e)
	require.Equal(t, before, after)
	_, e = s.SaveSystemTableRetention(ctx, repo.ProbeRetentionID, v.Revision, v.Policy)
	require.Equal(t, "STORAGE_CONTROL_PRECONDITION_REQUIRED", dto.CodeOf(e))
	_, e = s.SystemTableRetention(context.Background(), repo.ProbeRetentionID)
	require.ErrorIs(t, e, ErrRootRequired)
}
func TestSystemRetentionControlNativeAtomicCleanupCrossPreviewAndReceipts(t *testing.T) {
	s, apply, db := systemRetentionNative(t)
	ctx := controlRoot()
	for _, id := range []string{repo.ProbeRetentionID, repo.DiscoveryRetentionID} {
		adoptSystemNative(t, s, apply, id)
		enableSystemNative(t, s, apply, id, 1)
	}
	seedSystemNative(t, db)
	pool, _ := db.DB()
	pool.SetMaxOpenConns(1)
	preview, e := s.PreviewSystemTableRetention(ctx, repo.ProbeRetentionID, 2)
	require.NoError(t, e)
	require.EqualValues(t, 1, preview.Rows)
	settings := SystemTableRetentionSettings{30, 100}
	_, e = s.ChangeSystemTableRetention(ctx, repo.DiscoveryRetentionID, SystemTableRetentionChange{RequestID: uuid.NewString(), ExpectedRevision: 2, Action: "save", Settings: &settings, Confirm: true})
	require.NoError(t, e)
	key := uuid.NewString()
	r, e := s.RunSystemTableRetention(ctx, repo.ProbeRetentionID, 2, key, preview.ID)
	require.NoError(t, e)
	require.EqualValues(t, 1, r.Rows)
	require.Equal(t, [2]int64{4, 4}, systemRows(t, db))
	again, e := s.RunSystemTableRetention(ctx, repo.ProbeRetentionID, 2, key, preview.ID)
	require.NoError(t, e)
	require.Equal(t, r.Rows, again.Rows)
	receipt, e := s.SystemTableRetentionRunRequest(ctx, repo.ProbeRetentionID, key)
	require.NoError(t, e)
	require.Equal(t, r.Rows, receipt.Rows)
	_, e = s.RunSystemTableRetention(ctx, repo.ProbeRetentionID, 2, uuid.NewString(), preview.ID)
	require.Equal(t, "SYSTEM_RETENTION_PREVIEW_EXPIRED", dto.CodeOf(e))
	apply()
	enableSystemNative(t, s, apply, repo.DiscoveryRetentionID, 3)
	_, e = s.RunSystemTableRetention(ctx, repo.DiscoveryRetentionID, 4, uuid.NewString(), preview.ID)
	require.Equal(t, "SYSTEM_RETENTION_PREVIEW_EXPIRED", dto.CodeOf(e))
	preview, e = s.PreviewSystemTableRetention(ctx, repo.DiscoveryRetentionID, 4)
	require.NoError(t, e)
	require.EqualValues(t, 1, preview.Rows)
	_, e = s.RunSystemTableRetention(ctx, repo.DiscoveryRetentionID, 4, uuid.NewString(), preview.ID)
	require.NoError(t, e)
	require.Equal(t, [2]int64{4, 3}, systemRows(t, db))
}
func TestSystemRetentionControlNativeReceiptFailureRollsBackAndExpires(t *testing.T) {
	s, apply, db := systemRetentionNative(t)
	ctx := controlRoot()
	adoptSystemNative(t, s, apply, repo.ProbeRetentionID)
	enableSystemNative(t, s, apply, repo.ProbeRetentionID, 1)
	seedSystemNative(t, db)
	preview, e := s.PreviewSystemTableRetention(ctx, repo.ProbeRetentionID, 2)
	require.NoError(t, e)
	key := uuid.NewString()
	name := "owned_system_receipt_failure"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if row, ok := tx.Statement.Dest.(*m.CleanupRun); ok && row.RunID == key {
			tx.AddError(errors.New("owned rollback receipt"))
		}
	}))
	_, e = s.RunSystemTableRetention(ctx, repo.ProbeRetentionID, 2, key, preview.ID)
	require.Error(t, e)
	require.Equal(t, [2]int64{5, 4}, systemRows(t, db))
	require.NoError(t, db.Callback().Create().Remove(name))
	row, e := s.systemRetentionLedger(repo.ProbeRetentionID).Get(ctx, db, preview.ID)
	require.NoError(t, e)
	require.Equal(t, "preview", row.State)
	_, e = s.SystemTableRetentionRunRequest(ctx, repo.ProbeRetentionID, key)
	require.Equal(t, "SYSTEM_RETENTION_RUN_NOT_FOUND", dto.CodeOf(e))
	s.opts.Now = func() time.Time { return preview.At.Add(6 * time.Minute) }
	_, e = s.RunSystemTableRetention(ctx, repo.ProbeRetentionID, 2, uuid.NewString(), preview.ID)
	require.Equal(t, "SYSTEM_RETENTION_PREVIEW_EXPIRED", dto.CodeOf(e))
}
func TestSystemRetentionControlNativeTamperDatabaseRollbackAndPendingPause(t *testing.T) {
	s, apply, db := systemRetentionNative(t)
	ctx := controlRoot()
	id := repo.ProbeRetentionID
	adoptSystemNative(t, s, apply, id)
	enableSystemNative(t, s, apply, id, 1)
	seedSystemNative(t, db)
	preview, e := s.PreviewSystemTableRetention(ctx, id, 2)
	require.NoError(t, e)
	no := false
	_, e = s.ChangeSystemTableRetention(ctx, id, SystemTableRetentionChange{RequestID: uuid.NewString(), ExpectedRevision: 2, Action: "status", Enabled: &no, Confirm: true})
	require.NoError(t, e)
	_, e = s.RunSystemTableRetention(ctx, id, 2, uuid.NewString(), preview.ID)
	require.Equal(t, "SYSTEM_RETENTION_PUBLICATION_PENDING", dto.CodeOf(e))
	require.Equal(t, [2]int64{5, 4}, systemRows(t, db))
	apply()
	_, projection := systemRetentionFiles(id)
	file := filepath.Join(s.opts.Budget.ChangeDirectory(), projection+".json")
	raw, e := os.ReadFile(file)
	require.NoError(t, e)
	var snap snapshotPolicyProjection
	require.NoError(t, json.Unmarshal(raw, &snap))
	snap.Digest = "tampered"
	bad, _ := json.Marshal(snap)
	require.NoError(t, os.WriteFile(file, bad, 0600))
	v, e := s.SystemTableRetention(ctx, id)
	require.NoError(t, e)
	require.Equal(t, "unavailable", v.Configuration.Authority)
	require.False(t, v.Configuration.Ready)
	_, e = s.PreviewSystemTableRetention(ctx, id, 3)
	require.Error(t, e)
	require.NoError(t, os.WriteFile(file, raw, 0600))
	require.NoError(t, db.Where("tenant_id = ? AND scope_id = ? AND domain = ?", 0, s.opts.Control.opts.Scope.ID(), systemRetentionDomain(id)).Delete(&m.ControlConfiguration{}).Error)
	require.NoError(t, os.Remove(file))
	v, e = s.SystemTableRetention(ctx, id)
	require.NoError(t, e)
	require.Equal(t, "SYSTEM_RETENTION_RESTORE_CONFIRMATION_REQUIRED", v.Configuration.Problem)
}
func TestSystemRetentionControlNativeLegacyUnknownAndPerAdapterFingerprint(t *testing.T) {
	s, apply, _ := systemRetentionNative(t)
	ctx := controlRoot()
	id := repo.ProbeRetentionID
	adoptSystemNative(t, s, apply, id)
	enableSystemNative(t, s, apply, id, 1)
	_, e := s.opts.Budget.Document(ctx, "system-table-retention", func(raw []byte) ([]byte, error) {
		d, e := s.systemTableDocument(raw)
		if e != nil {
			return nil, e
		}
		d.Revision++
		d.Runs = append(d.Runs, SystemTableRetentionRun{ID: uuid.NewString(), TableID: repo.DiscoveryRetentionID, State: "unconfirmed", At: time.Now(), Cutoff: time.Now().AddDate(0, 0, -30), Policy: d.Policies[repo.DiscoveryRetentionID]})
		return json.Marshal(d)
	})
	require.NoError(t, e)
	probe, e := s.SystemTableRetention(ctx, id)
	require.NoError(t, e)
	require.True(t, probe.Configuration.ExecutionReady, "other adapter's legacy evidence must not revoke this policy")
	cache, e := s.SystemTableRetention(ctx, repo.DiscoveryRetentionID)
	require.NoError(t, e)
	_, e = s.AdoptSystemTableRetention(ctx, repo.DiscoveryRetentionID, SystemTableRetentionAdoption{uuid.NewString(), cache.Configuration.LegacyDigest, true})
	require.Equal(t, "SYSTEM_RETENTION_LEGACY_RESULT_UNCONFIRMED", dto.CodeOf(e))
	_, e = s.PreviewSystemTableRetention(ctx, "event_task_histories", 1)
	require.ErrorIs(t, e, ErrSystemTableNotFound)
}

func TestSystemRetentionControlNativeExternalReferenceAndIndexGate(t *testing.T) {
	s, apply, db := systemRetentionNative(t)
	id := repo.ProbeRetentionID
	ctx := controlRoot()
	adoptSystemNative(t, s, apply, id)
	enableSystemNative(t, s, apply, id, 1)
	seedSystemNative(t, db)
	preview, e := s.PreviewSystemTableRetention(ctx, id, 2)
	require.NoError(t, e)
	table := capm.HealthProbeResult{}.TableName()
	child := table + "_owned_reference"
	require.NoError(t, db.Exec("CREATE TABLE "+child+" (adapter_id varchar(512), probe_window_start timestamptz, FOREIGN KEY(adapter_id,probe_window_start) REFERENCES "+table+"(adapter_id,probe_window_start) ON DELETE CASCADE)").Error)
	require.NoError(t, db.Exec("INSERT INTO "+child+" SELECT adapter_id,probe_window_start FROM "+table+" ORDER BY probe_window_start LIMIT 1").Error)
	_, e = s.RunSystemTableRetention(ctx, id, 2, uuid.NewString(), preview.ID)
	require.ErrorContains(t, e, "external_references")
	require.Equal(t, [2]int64{5, 4}, systemRows(t, db))
	require.NoError(t, db.Exec("DROP TABLE "+child).Error)
	require.NoError(t, db.Exec("DROP INDEX "+strings.Split(table, ".")[0]+".idx_registry_probe_retention").Error)
	_, e = s.PreviewSystemTableRetention(ctx, id, 2)
	require.ErrorContains(t, e, "index_required")
	require.Equal(t, [2]int64{5, 4}, systemRows(t, db))
}
