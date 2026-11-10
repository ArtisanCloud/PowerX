package storagegov

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func adoptBudgetNative(t *testing.T, s *Service) (HostBudgetMutation, HostBudgetAdoption) {
	t.Helper()
	c, e := s.HostBudgetConfiguration(controlRoot())
	require.NoError(t, e)
	in := HostBudgetAdoption{uuid.NewString(), c.LegacyRevision, c.LegacyDigest, true}
	out, e := s.AdoptHostBudget(controlRoot(), in)
	require.NoError(t, e)
	return out, in
}
func TestHostBudgetNativePostgresAdoptionCASAndReceipts(t *testing.T) {
	s, c, q, db := monitoringNative(t)
	ctx := controlRoot()
	before, e := s.opts.Budget.ControlSnapshot(ctx)
	require.NoError(t, e)
	lease, e := s.opts.Budget.Reserve(ctx, "owned-before-adoption", 1, time.Minute)
	require.NoError(t, e)
	_, e = s.SaveRuntimeBudget(ctx, 1, before.Legacy)
	require.Equal(t, "STORAGE_CONTROL_PRECONDITION_REQUIRED", dto.CodeOf(e))
	out, adopt := adoptBudgetNative(t, s)
	require.Equal(t, "pending", out.Configuration.State)
	require.Equal(t, budgetSettings(before.Legacy), out.OperationPolicy)
	replay, e := s.AdoptHostBudget(ctx, adopt)
	require.NoError(t, e)
	require.Equal(t, out.OperationRevision, replay.OperationRevision)
	applyMonitoring(t, c, q)
	after, e := s.HostBudgetConfiguration(ctx)
	require.NoError(t, e)
	require.Equal(t, "applied", after.State)
	require.EqualValues(t, 1, *after.SnapshotRevision)
	snap, e := s.opts.Budget.ControlSnapshot(ctx)
	require.NoError(t, e)
	require.Equal(t, before.Legacy, snap.Legacy)
	require.NoError(t, lease.Release(ctx))
	in := HostBudgetChange{uuid.NewString(), 1, *after.DesiredPolicy, true}
	in.Settings.MaxConcurrent = 2
	saved, e := s.SaveHostBudget(ctx, in)
	require.NoError(t, e)
	again, e := s.SaveHostBudget(ctx, in)
	require.NoError(t, e)
	require.Equal(t, saved.OperationRevision, again.OperationRevision)
	in.Settings.MaxConcurrent = 3
	_, e = s.SaveHostBudget(ctx, in)
	require.Equal(t, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT", dto.CodeOf(e))
	applyMonitoring(t, c, q)
	start := make(chan struct{})
	results := make(chan error, 6)
	var wg sync.WaitGroup
	for n := 0; n < 6; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			<-start
			p := *after.DesiredPolicy
			p.MaxConcurrent = n + 2
			_, e := s.SaveHostBudget(ctx, HostBudgetChange{uuid.NewString(), 2, p, true})
			results <- e
		}(n)
	}
	close(start)
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
	var count int64
	require.NoError(t, db.Model(&m.ControlRevision{}).Where("domain = ?", ControlBudget).Count(&count).Error)
	require.EqualValues(t, 3, count)
	_, e = s.HostBudgetRequest(context.Background(), adopt.RequestID)
	require.ErrorIs(t, e, ErrRootRequired)
}
func TestHostBudgetNativePostgresPendingLimitsAndReleaseWithoutDatabase(t *testing.T) {
	s, c, q, db := monitoringNative(t)
	ctx := controlRoot()
	adoptBudgetNative(t, s)
	applyMonitoring(t, c, q)
	state, e := s.HostBudgetConfiguration(ctx)
	require.NoError(t, e)
	original := *state.DesiredPolicy
	tight := original
	tight.MinFreeBytes = 1 << 49
	_, e = s.SaveHostBudget(ctx, HostBudgetChange{uuid.NewString(), 1, tight, true})
	require.NoError(t, e)
	_, e = s.opts.Budget.Reserve(ctx, "owned-tight", 0, time.Minute)
	require.ErrorIs(t, e, budget.ErrInsufficient, "pending tightening applies before publication")
	applyMonitoring(t, c, q)
	_, e = s.SaveHostBudget(ctx, HostBudgetChange{uuid.NewString(), 2, original, true})
	require.NoError(t, e)
	_, e = s.opts.Budget.Reserve(ctx, "owned-relax-pending", 0, time.Minute)
	require.ErrorIs(t, e, budget.ErrInsufficient, "pending relaxation cannot enlarge allowance")
	applyMonitoring(t, c, q)
	pool, e := db.DB()
	require.NoError(t, e)
	pool.SetMaxOpenConns(1)
	lease, e := s.opts.Budget.Reserve(ctx, "owned-one-connection", 1, time.Minute)
	require.NoError(t, e)
	require.NoError(t, db.Migrator().DropTable(&m.ControlConfiguration{}))
	_, e = s.opts.Budget.Reserve(ctx, "owned-outage", 1, time.Minute)
	require.Error(t, e)
	require.Error(t, lease.Progress(ctx, 0, 2, time.Minute))
	require.NoError(t, lease.Release(ctx), "release must remain available while authority is down")
	snap, e := s.opts.Budget.ControlSnapshot(ctx)
	require.NoError(t, e)
	require.NotNil(t, snap.Projection)
}
func TestHostBudgetNativePostgresSharedOwnerAndCompetingAdoption(t *testing.T) {
	s, c, q, db := monitoringNative(t)
	ctx := controlRoot()
	dir := s.opts.Budget.ChangeDirectory()
	otherBudget, e := budget.New(budget.Config{Directory: dir, FilesystemPath: s.opts.FilesystemPath, HostID: "owned-host", FilesystemID: "owned-fs", InstanceID: "owned-other", MinFreeBytes: 1 << 20, CriticalFreeBytes: 1})
	require.NoError(t, e)
	otherControl, e := NewControlService(db, ControlOptions{Scope: ControlScope{"owned-host", "owned-fs", "owned-other"}, Queue: q})
	require.NoError(t, e)
	other, e := NewService(Options{Identity: Identity{HostID: "owned-host", FilesystemID: "owned-fs", InstanceID: "owned-other", Environment: "test"}, FilesystemPath: s.opts.FilesystemPath, Collector: &fixtureCollector{}, DB: db, Budget: otherBudget, Control: otherControl, ControlWorkerEnabled: true})
	require.NoError(t, e)
	adoptBudgetNative(t, s)
	adoptBudgetNative(t, other)
	applyMonitoring(t, c, q)
	items, e := q.Dequeue(ctx, controlQueueTenant, otherControl.subscriber(), "owned-budget", "owned-loser", 1, time.Millisecond)
	require.NoError(t, e)
	require.Len(t, items, 1)
	require.Error(t, otherControl.ApplyDelivery(ctx, items[0]), "second installation cannot replace the host owner")
	config, e := other.HostBudgetConfiguration(ctx)
	require.NoError(t, e)
	require.Equal(t, "owned-install", config.OwnerInstance)
	require.False(t, config.CanManage)
	_, e = other.SaveHostBudget(ctx, HostBudgetChange{uuid.NewString(), config.DesiredRevision, *config.DesiredPolicy, true})
	require.Equal(t, "STORAGE_BUDGET_OWNER_REQUIRED", dto.CodeOf(e))
	first, e := s.opts.Budget.Reserve(ctx, "owned-main", 1, time.Minute)
	require.NoError(t, e)
	_, e = otherBudget.Reserve(ctx, "owned-other-busy", 1, time.Minute)
	require.ErrorIs(t, e, budget.ErrBusy)
	require.NoError(t, first.Release(ctx))
	second, e := otherBudget.Reserve(ctx, "owned-other", 1, time.Minute)
	require.NoError(t, e)
	require.NoError(t, second.Release(ctx))
	standalone, e := budget.New(budget.Config{Directory: dir, FilesystemPath: s.opts.FilesystemPath, HostID: "owned-host", FilesystemID: "owned-fs", InstanceID: "owned-worker", MinFreeBytes: 1 << 20, CriticalFreeBytes: 1})
	require.NoError(t, e)
	_, e = standalone.Reserve(ctx, "owned-worker", 1, time.Minute)
	require.ErrorIs(t, e, budget.ErrControlRequired)
	snap, e := standalone.ControlSnapshot(ctx)
	require.NoError(t, e)
	_, e = standalone.SavePolicy(ctx, snap.Legacy.Revision, snap.Legacy)
	require.ErrorIs(t, e, budget.ErrControlRequired)
}
func TestHostBudgetNativePostgresTamperedProjectionFailsClosed(t *testing.T) {
	s, c, q, _ := monitoringNative(t)
	ctx := controlRoot()
	adoptBudgetNative(t, s)
	applyMonitoring(t, c, q)
	path := filepath.Join(s.opts.Budget.ChangeDirectory(), "budget.json")
	raw, e := os.ReadFile(path)
	require.NoError(t, e)
	var ledger map[string]any
	require.NoError(t, json.Unmarshal(raw, &ledger))
	ledger["control"].(map[string]any)["digest"] = strings.Repeat("0", 64)
	raw, e = json.Marshal(ledger)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(path, raw, 0600))
	_, e = s.opts.Budget.Reserve(ctx, "owned-tamper", 0, time.Minute)
	require.Error(t, e)
	out, e := s.HostBudgetConfiguration(ctx)
	require.Error(t, e)
	require.Equal(t, "unconfirmed", out.State)
	require.Nil(t, out.EffectivePolicy)
}

func TestHostBudgetNativePostgresPublicationRecoveryDoesNotReplayMutation(t *testing.T) {
	s, c, q, db := monitoringNative(t)
	ctx := controlRoot()
	adoptBudgetNative(t, s)
	// 原投递已经入队，再恢复同一持久任务，不能触发信封幂等冲突。
	require.NoError(t, q.Recover(ctx, controlQueueTenant, c.subscriber()))
	applyMonitoring(t, c, q)
	state, e := s.HostBudgetConfiguration(ctx)
	require.NoError(t, e)
	require.EqualValues(t, 1, state.AppliedRevision)
	rows, e := q.Dequeue(ctx, controlQueueTenant, c.subscriber(), "owned-monitoring", "owned-worker", 10, time.Millisecond)
	require.NoError(t, e)
	require.Empty(t, rows, "terminal task copies cannot apply a second configuration")
	var count int64
	require.NoError(t, db.Model(&m.ControlRevision{}).Where("domain = ?", ControlBudget).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestHostBudgetNativePostgresLostProjectionCannotRevertOtherInstancesToLegacy(t *testing.T) {
	s, c, q, db := monitoringNative(t)
	ctx := controlRoot()
	adoptBudgetNative(t, s)
	applyMonitoring(t, c, q)
	cfg := budget.Config{Directory: s.opts.Budget.ChangeDirectory(), FilesystemPath: s.opts.FilesystemPath, HostID: "owned-host", FilesystemID: "owned-fs", InstanceID: "owned-secondary", MinFreeBytes: 1 << 20, CriticalFreeBytes: 1}
	secondary, e := budget.New(cfg)
	require.NoError(t, e)
	ctrl, e := NewControlService(db, ControlOptions{Scope: ControlScope{"owned-host", "owned-fs", "owned-secondary"}, Queue: q})
	require.NoError(t, e)
	_, e = NewService(Options{Identity: Identity{HostID: "owned-host", FilesystemID: "owned-fs", InstanceID: "owned-secondary", Environment: "test"}, FilesystemPath: s.opts.FilesystemPath, Collector: &fixtureCollector{}, DB: db, Budget: secondary, Control: ctrl, ControlWorkerEnabled: true})
	require.NoError(t, e)
	lease, e := secondary.Reserve(ctx, "owned-secondary", 1, time.Minute)
	require.NoError(t, e)
	path := filepath.Join(cfg.Directory, "budget.json")
	raw, e := os.ReadFile(path)
	require.NoError(t, e)
	var state map[string]any
	require.NoError(t, json.Unmarshal(raw, &state))
	delete(state, "control")
	raw, e = json.Marshal(state)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(path, raw, 0600))
	_, e = secondary.Reserve(ctx, "owned-fallback", 1, time.Minute)
	require.ErrorIs(t, e, budget.ErrControlRequired)
	require.NoError(t, lease.Release(ctx), "lost projection must not prevent release")
	unbound, e := budget.New(cfg)
	require.NoError(t, e)
	_, e = unbound.Reserve(ctx, "owned-worker-fallback", 1, time.Minute)
	require.ErrorIs(t, e, budget.ErrControlRequired)
	_, e = unbound.SavePolicy(ctx, 1, budget.BudgetPolicy{Revision: 1, MinFreeBytes: 1 << 20, CriticalFreeBytes: 1, MaxConcurrent: 1})
	require.ErrorIs(t, e, budget.ErrControlRequired)
}
