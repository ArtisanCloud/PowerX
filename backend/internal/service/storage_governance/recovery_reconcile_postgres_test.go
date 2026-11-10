package storagegov

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	mevent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func restoreConfiguration(t *testing.T, db *gorm.DB, old m.ControlConfiguration) {
	t.Helper()
	require.NoError(t, db.Model(&m.ControlConfiguration{}).Where("tenant_id=0 AND scope_id=? AND domain=?", old.ScopeID, old.Domain).Updates(map[string]any{"revision": old.Revision, "applied_revision": old.AppliedRevision, "publication_state": old.PublicationState, "document": old.Document, "digest": old.Digest, "actor_id": old.ActorID}).Error)
}
func finishOwnedRecovery(t *testing.T, s *Service, apply func(), epoch string, revision int64) ExecutionMutation {
	t.Helper()
	out, e := s.ChangeExecution(controlRoot(), prepareRecovery(revision, epoch))
	require.NoError(t, e)
	apply()
	state, e := s.ExecutionConfiguration(controlRoot())
	require.NoError(t, e)
	require.True(t, state.Recovery.Prepared)
	for _, resource := range state.Recovery.Resources {
		require.Equal(t, "ready", resource.State, "%+v", resource)
	}
	_, e = s.ChangeExecution(controlRoot(), acknowledgeRecovery(out.OperationRevision, epoch))
	require.NoError(t, e)
	apply()
	state, e = s.ExecutionConfiguration(controlRoot())
	require.NoError(t, e)
	require.Equal(t, "paused", state.Mode)
	return out
}

func TestRecoveryReconcileNativePostgresDatabaseRollbackRebasesAllVersionsAndKeepsBudgetStrict(t *testing.T) {
	s, c, q, db := monitoringNative(t)
	ctx := controlRoot()
	store, _ := ownedRecoveryAnchor(t, s)
	legacy, e := s.DatabaseTableMonitoring(ctx)
	require.NoError(t, e)
	_, e = s.AdoptTableMonitoring(ctx, legacy.Policy.Revision, legacy.Configuration.LegacyDigest, uuid.NewString())
	require.NoError(t, e)
	adoptBudgetNative(t, s)
	applyMonitoring(t, c, q)
	var old []m.ControlConfiguration
	require.NoError(t, db.Order("domain").Find(&old).Error)
	conf, e := s.HostBudgetConfiguration(ctx)
	require.NoError(t, e)
	tight := *conf.DesiredPolicy
	tight.MinFreeBytes *= 2
	_, e = s.SaveHostBudget(ctx, HostBudgetChange{uuid.NewString(), 1, tight, true})
	require.NoError(t, e)
	mon, e := s.DatabaseTableMonitoring(ctx)
	require.NoError(t, e)
	p := *mon.Configuration.DesiredPolicy
	p.Enabled = true
	_, e = s.SaveTableMonitoringConfiguration(ctx, 1, uuid.NewString(), p)
	require.NoError(t, e)
	applyMonitoring(t, c, q)
	for _, row := range old {
		restoreConfiguration(t, db, row)
	}
	epoch, e := store.Begin(ctx, uuid.NewString(), uuid.NewString())
	require.NoError(t, e)
	input := prepareRecovery(0, epoch.Epoch)
	out, e := s.ChangeExecution(ctx, input)
	require.NoError(t, e)
	replay, e := s.ChangeExecution(ctx, input)
	require.NoError(t, e)
	require.Equal(t, out.OperationRevision, replay.OperationRevision)
	var rows []m.ControlConfiguration
	require.NoError(t, db.Find(&rows).Error)
	for _, row := range rows {
		if row.Domain == string(ControlExecution) {
			continue
		}
		require.EqualValues(t, 3, row.Revision)
		stamp := recoveryStamp(row.Document)
		require.NotNil(t, stamp)
		require.EqualValues(t, 1, stamp.PreviousRevision)
		require.Equal(t, epoch.Epoch, stamp.Epoch)
	}
	applyMonitoring(t, c, q)
	conf, e = s.HostBudgetConfiguration(ctx)
	require.NoError(t, e)
	require.Equal(t, tight.MinFreeBytes, conf.DesiredPolicy.MinFreeBytes)
	// 相同版本、篡改预算正文的投影不能被确认。
	path := filepath.Join(s.opts.Budget.ChangeDirectory(), "budget.json")
	raw, e := os.ReadFile(path)
	require.NoError(t, e)
	var ledger map[string]any
	require.NoError(t, json.Unmarshal(raw, &ledger))
	ledger["control"].(map[string]any)["digest"] = controlDigest([]byte("tamper"))
	changed, _ := json.Marshal(ledger)
	require.NoError(t, os.WriteFile(path, changed, 0600))
	_, e = s.ChangeExecution(ctx, acknowledgeRecovery(out.OperationRevision, epoch.Epoch))
	require.Error(t, e)
	require.NoError(t, os.WriteFile(path, raw, 0600))
	_, e = s.ChangeExecution(ctx, acknowledgeRecovery(out.OperationRevision, epoch.Epoch))
	require.NoError(t, e)
	applyMonitoring(t, c, q)
	// 后续编辑必须延续本代次来源，旧审计仍可读取。
	mon, e = s.DatabaseTableMonitoring(ctx)
	require.NoError(t, e)
	p = *mon.Configuration.DesiredPolicy
	p.Enabled = false
	_, e = s.SaveTableMonitoringConfiguration(ctx, 3, uuid.NewString(), p)
	require.NoError(t, e)
	latest, e := c.repo.Revision(ctx, c.opts.Scope.ID(), string(ControlTableMonitoring), 4)
	require.NoError(t, e)
	require.Equal(t, epoch.Epoch, recoveryStamp(latest.Document).Epoch)
	_, e = c.repo.Revision(ctx, c.opts.Scope.ID(), string(ControlTableMonitoring), 2)
	require.NoError(t, e)
}

func TestRecoveryReconcileNativePostgresMissingResourceAndRegistrationFailureAreAtomic(t *testing.T) {
	for _, mode := range []string{"missing-resource", "task-registration"} {
		t.Run(mode, func(t *testing.T) {
			s, c, q, db := monitoringNative(t)
			ctx := controlRoot()
			store, _ := ownedRecoveryAnchor(t, s)
			adoptBudgetNative(t, s)
			applyMonitoring(t, c, q)
			var old m.ControlConfiguration
			require.NoError(t, db.First(&old).Error)
			epoch, e := store.Begin(ctx, uuid.NewString(), uuid.NewString())
			require.NoError(t, e)
			if mode == "missing-resource" {
				require.NoError(t, db.Unscoped().Delete(&old).Error)
			} else {
				require.NoError(t, db.Migrator().DropTable(&mevent.TaskHistory{}))
			}
			var before int64
			require.NoError(t, db.Model(&m.ControlRevision{}).Count(&before).Error)
			_, e = s.ChangeExecution(ctx, prepareRecovery(0, epoch.Epoch))
			require.Error(t, e)
			if mode == "missing-resource" {
				require.Equal(t, "STORAGE_RECOVERY_RESOURCE_MISSING", dto.CodeOf(e))
			} else {
				var after m.ControlConfiguration
				require.NoError(t, db.First(&after).Error)
				require.Equal(t, old.Document, after.Document)
				require.Equal(t, old.Revision, after.Revision)
			}
			var after int64
			require.NoError(t, db.Model(&m.ControlRevision{}).Count(&after).Error)
			require.Equal(t, before, after)
		})
	}
}

func TestRecoveryReconcileNativePostgresResourceAdapters(t *testing.T) {
	t.Run("lifecycle", func(t *testing.T) {
		s, c, q, _, _ := lifecycleNative(t)
		settings := adoptNativeLifecycle(t, s, c, q)
		settings.Enabled = true
		_, e := s.SaveLifecycleConfiguration(controlRoot(), "logs", 1, uuid.NewString(), settings)
		require.NoError(t, e)
		applyMonitoring(t, c, q)
		store, _ := ownedRecoveryAnchor(t, s)
		epoch, e := store.Begin(controlRoot(), uuid.NewString(), uuid.NewString())
		require.NoError(t, e)
		finishOwnedRecovery(t, s, func() { applyMonitoring(t, c, q) }, epoch.Epoch, 0)
		conf, e := s.LifecycleConfiguration(controlRoot(), "logs")
		require.NoError(t, e)
		require.False(t, conf.DesiredPolicy.Enabled)
	})
	t.Run("archive-policy-entry-watermark", func(t *testing.T) {
		ac, c, _, _ := policyNative(t)
		adoptNativePolicy(t, ac, c)
		s := ac.service
		store, _ := ownedRecoveryAnchor(t, s)
		epoch, e := store.Begin(controlRoot(), uuid.NewString(), uuid.NewString())
		require.NoError(t, e)
		finishOwnedRecovery(t, s, func() { applyMonitoring(t, c, c.opts.Queue) }, epoch.Epoch, 0)
		conf, e := ac.Configuration(controlRoot())
		require.NoError(t, e)
		require.Greater(t, conf.Policies[0].Revision, int64(8))
		require.False(t, conf.Policies[0].Enabled)
		require.False(t, conf.Policies[0].RemoteCleanupEnabled)
	})
	t.Run("snapshot", func(t *testing.T) {
		s, c, q, _, snap, _, _ := snapshotNative(t)
		bindAndAdoptSnapshot(t, s, snap)
		applyMonitoring(t, c, q)
		settings := snapshotSettings()
		out := snapshotChange(t, snap, "save", "", &settings, nil)
		applyMonitoring(t, c, q)
		snapshotChange(t, snap, "current", out.PolicyID, nil, nil)
		applyMonitoring(t, c, q)
		yes := true
		snapshotChange(t, snap, "status", out.PolicyID, nil, &yes)
		applyMonitoring(t, c, q)
		store, _ := ownedRecoveryAnchor(t, s)
		epoch, e := store.Begin(controlRoot(), uuid.NewString(), uuid.NewString())
		require.NoError(t, e)
		finishOwnedRecovery(t, s, func() { applyMonitoring(t, c, q) }, epoch.Epoch, 0)
	})
	t.Run("backup", func(t *testing.T) {
		s, c, q, _, policies, _, _ := backupNative(t)
		adoptBackup(t, policies)
		applyMonitoring(t, c, q)
		settings := backupSettings()
		out := backupChange(t, policies, "save", "", &settings, nil)
		applyMonitoring(t, c, q)
		yes := true
		backupChange(t, policies, "status", out.PolicyUUID, nil, &yes)
		applyMonitoring(t, c, q)
		backupChange(t, policies, "current", out.PolicyUUID, nil, nil)
		applyMonitoring(t, c, q)
		store, _ := ownedRecoveryAnchor(t, s)
		epoch, e := store.Begin(controlRoot(), uuid.NewString(), uuid.NewString())
		require.NoError(t, e)
		finishOwnedRecovery(t, s, func() { applyMonitoring(t, c, q) }, epoch.Epoch, 0)
		conf, e := policies.ControlConfiguration(controlRoot())
		require.NoError(t, e)
		for _, p := range conf.Policies {
			require.False(t, p.Enabled)
			require.False(t, p.Current)
			require.False(t, p.Settings.DrillEnabled)
		}
	})
}

func TestRecoveryReconcileNativePostgresLegacyAndChangedSourceCannotAcknowledge(t *testing.T) {
	s, apply, db := systemRetentionNative(t)
	ctx := controlRoot()
	seedSystemNative(t, db)
	adoptSystemNative(t, s, apply, repo.ProbeRetentionID)
	store, _ := ownedRecoveryAnchor(t, s)
	epoch, e := store.Begin(ctx, uuid.NewString(), uuid.NewString())
	require.NoError(t, e)
	// 部署来源变更不能由恢复操作自动接受。
	dialector := s.opts.DB.Dialector.(*postgres.Dialector)
	original := dialector.Config.DSN
	dialector.Config.DSN = original + " "
	defer func() { dialector.Config.DSN = original }()
	_, e = s.ChangeExecution(ctx, prepareRecovery(0, epoch.Epoch))
	require.Equal(t, "STORAGE_RECOVERY_SOURCE_CHANGED", dto.CodeOf(e))
}

func TestRecoveryReconcileNativePostgresTargetVerificationAndLegacyInventoryBlockConfirmation(t *testing.T) {
	for _, adopt := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy-target", true: "reverify-target"}[adopt], func(t *testing.T) {
			ac, c, _, _ := targetNative(t)
			s := ac.service
			ctx := controlRoot()
			if adopt {
				adoptNativeTarget(t, ac, c)
			}
			store, _ := ownedRecoveryAnchor(t, s)
			epoch, e := store.Begin(ctx, uuid.NewString(), uuid.NewString())
			require.NoError(t, e)
			out, e := s.ChangeExecution(ctx, prepareRecovery(0, epoch.Epoch))
			require.NoError(t, e)
			applyMonitoring(t, c, c.opts.Queue)
			pool, e := s.opts.DB.DB()
			require.NoError(t, e)
			pool.SetMaxOpenConns(1)
			state, e := s.ExecutionConfiguration(ctx)
			require.NoError(t, e)
			require.Len(t, state.Recovery.Resources, 1)
			expected := "STORAGE_RECOVERY_LEGACY_REQUIRED"
			if adopt {
				expected = "STORAGE_RECOVERY_TARGET_REQUIRED"
			}
			require.Equal(t, expected, state.Recovery.Resources[0].Problem)
			_, e = s.ChangeExecution(ctx, acknowledgeRecovery(out.OperationRevision, epoch.Epoch))
			require.Error(t, e)
		})
	}
}

func TestRecoveryReconcileNativePostgresConcurrentReplayCreatesOneAtomicSet(t *testing.T) {
	s, c, q, db := monitoringNative(t)
	ctx := controlRoot()
	adoptBudgetNative(t, s)
	applyMonitoring(t, c, q)
	store, _ := ownedRecoveryAnchor(t, s)
	epoch, e := store.Begin(ctx, uuid.NewString(), uuid.NewString())
	require.NoError(t, e)
	input := prepareRecovery(0, epoch.Epoch)
	start := make(chan struct{})
	errs := make(chan error, 6)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; _, e := s.ChangeExecution(ctx, input); errs <- e }()
	}
	close(start)
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	var count int64
	require.NoError(t, db.Model(&m.ControlRevision{}).Count(&count).Error)
	require.EqualValues(t, 3, count, "one original budget + one rebased budget + one execution receipt")
	applyMonitoring(t, c, q)
	pool, e := db.DB()
	require.NoError(t, e)
	pool.SetMaxOpenConns(1)
	_, e = s.ChangeExecution(ctx, acknowledgeRecovery(1, epoch.Epoch))
	require.NoError(t, e)
}

func TestRecoveryReconcileNativePostgresDatabaseAdaptersPreserveRowsAndPause(t *testing.T) {
	t.Run("invocation-retention", func(t *testing.T) {
		s, apply, db := retentionNative(t)
		seedRetentionNative(t, db)
		before := retentionRows(t, db)
		p := adoptRetentionNative(t, s, apply)
		enableRetentionNative(t, s, apply, p.Revision)
		store, _ := ownedRecoveryAnchor(t, s)
		epoch, e := store.Begin(controlRoot(), uuid.NewString(), uuid.NewString())
		require.NoError(t, e)
		finishOwnedRecovery(t, s, apply, epoch.Epoch, 0)
		conf, e := s.DatabaseRetention(controlRoot())
		require.NoError(t, e)
		require.False(t, conf.Policy.Enabled)
		require.Equal(t, before, retentionRows(t, db))
	})
	t.Run("both-system-tables", func(t *testing.T) {
		s, apply, db := systemRetentionNative(t)
		seedSystemNative(t, db)
		before := systemRows(t, db)
		for _, id := range []string{repo.ProbeRetentionID, repo.DiscoveryRetentionID} {
			adoptSystemNative(t, s, apply, id)
			enableSystemNative(t, s, apply, id, 1)
		}
		store, _ := ownedRecoveryAnchor(t, s)
		epoch, e := store.Begin(controlRoot(), uuid.NewString(), uuid.NewString())
		require.NoError(t, e)
		finishOwnedRecovery(t, s, apply, epoch.Epoch, 0)
		for _, id := range []string{repo.ProbeRetentionID, repo.DiscoveryRetentionID} {
			conf, e := s.SystemTableRetention(controlRoot(), id)
			require.NoError(t, e)
			require.False(t, conf.Policy.Enabled)
		}
		require.Equal(t, before, systemRows(t, db))
	})
}
