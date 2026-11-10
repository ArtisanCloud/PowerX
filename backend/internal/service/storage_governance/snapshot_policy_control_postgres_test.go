package storagegov

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	taskqueue "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	snapshot "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type ownedSnapshotBackend struct {
	mu   sync.Mutex
	wait func()
}

func (b *ownedSnapshotBackend) Capture(ctx context.Context, dest string) (string, error) {
	b.mu.Lock()
	wait := b.wait
	b.mu.Unlock()
	if wait != nil {
		wait()
	}
	for name, data := range map[string][]byte{"database/database.dump": []byte("owned synthetic dump"), "metadata/media-inventory.json": []byte(`[{"path":"database/database.dump","sha256":"` + controlDigest([]byte("owned synthetic dump")) + `","size":20}]`)} {
		path := filepath.Join(dest, name)
		if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			return "", e
		}
		if e := os.WriteFile(path, data, 0600); e != nil {
			return "", e
		}
	}
	return "owned-fixture-mvcc", nil
}
func (b *ownedSnapshotBackend) RestoreDatabase(context.Context, string) (string, string, error) {
	return "owned", "owned", nil
}
func snapshotNative(t *testing.T) (*Service, *ControlService, *taskqueue.StreamDriver, *gorm.DB, *snapshot.Service, *ownedSnapshotBackend, string) {
	t.Helper()
	s, c, q, db := monitoringNative(t)
	dir, e := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, e)
	program := filepath.Join(dir, "program")
	require.NoError(t, os.WriteFile(program, []byte("owned"), 0700))
	backend := &ownedSnapshotBackend{}
	snap := snapshot.NewService(context.Background(), snapshot.Config{Directory: filepath.Join(dir, "snapshots"), Instance: "owned-install", Version: "fixture", EffectiveConfig: []byte("owned: true"), Artifacts: map[string]string{"core/powerx": program}}, snapshot.Options{Backend: backend})
	t.Cleanup(snap.Close)
	return s, c, q, db, snap, backend, filepath.Join(dir, "snapshots")
}
func bindAndAdoptSnapshot(t *testing.T, s *Service, snap *snapshot.Service) snapshot.PolicyMutation {
	t.Helper()
	require.NoError(t, s.BindSystemSnapshotControl(snap))
	conf, e := snap.PolicyConfiguration(controlRoot())
	require.NoError(t, e)
	out, e := snap.AdoptPolicyConfiguration(controlRoot(), snapshot.PolicyAdoption{RequestID: uuid.NewString(), LegacyDigest: conf.LegacyDigest, Confirm: true})
	require.NoError(t, e)
	return out
}
func snapshotChange(t *testing.T, snap *snapshot.Service, action, id string, settings *snapshot.PolicySettings, enabled *bool) snapshot.PolicyMutation {
	t.Helper()
	conf, e := snap.PolicyConfiguration(controlRoot())
	require.NoError(t, e)
	var rev int64
	for _, p := range conf.Policies {
		if p.ID == id {
			rev = p.Revision
		}
	}
	out, e := snap.ChangePolicyConfiguration(controlRoot(), snapshot.PolicyChange{RequestID: uuid.NewString(), ExpectedRevision: conf.DesiredRevision, ExpectedPolicyRevision: rev, PolicyID: id, Action: action, Settings: settings, Enabled: enabled, Confirm: true})
	require.NoError(t, e)
	return out
}
func snapshotSettings() snapshot.PolicySettings {
	return snapshot.PolicySettings{Name: "Owned snapshot", IntervalMinutes: 60, RetentionDays: 7, RetentionCount: 1}
}
func TestSnapshotPolicyNativePostgresPausedAdoptionCASAndOriginalSource(t *testing.T) {
	s, c, q, db, snap, _, dir := snapshotNative(t)
	ctx := controlRoot()
	legacy, e := snap.PutPolicy(ctx, "", "legacy", snapshot.Policy{Enabled: true, IntervalMinutes: 60, RetentionDays: 7, RetentionCount: 2})
	require.NoError(t, e)
	before, e := os.ReadFile(filepath.Join(dir, "policies.json"))
	require.NoError(t, e)
	out := bindAndAdoptSnapshot(t, s, snap)
	require.False(t, out.Configuration.Policies[0].Enabled)
	require.False(t, out.Configuration.Policies[0].Current)
	applyMonitoring(t, c, q)
	after, e := os.ReadFile(filepath.Join(dir, "policies.json"))
	require.NoError(t, e)
	require.Equal(t, before, after)
	_, e = snap.PutPolicy(ctx, "", "bypass", snapshot.Policy{})
	require.Equal(t, "SNAPSHOT_POLICY_CONTROL_REQUIRED", dto.CodeOf(e))
	snapshotChange(t, snap, "current", legacy.ID, nil, nil)
	applyMonitoring(t, c, q)
	guard := &snapshotPolicyControl{s, snap}
	pool, _ := db.DB()
	pool.SetMaxOpenConns(1)
	called := false
	action := func(snapshot.PolicyEntry) error { called = true; return nil }
	require.NoError(t, guard.Guard(ctx, legacy.ID, 0, "manual", action))
	require.True(t, called)
	require.Equal(t, "SNAPSHOT_POLICY_DISABLED", dto.CodeOf(guard.Guard(ctx, legacy.ID, 0, "scheduled", action)))
	pool.SetMaxOpenConns(16)
	_, err := snap.StartForPolicy(ctx, uuid.NewString(), legacy.ID)
	require.Equal(t, "SNAPSHOT_POLICY_REVISION_REQUIRED", dto.CodeOf(err))
	_, err = snap.StartForPolicyRevision(ctx, uuid.NewString(), legacy.ID, 1)
	require.Equal(t, "SNAPSHOT_POLICY_CHANGED", dto.CodeOf(err))
	yes := true
	snapshotChange(t, snap, "status", legacy.ID, nil, &yes)
	called = false
	require.Error(t, guard.Guard(ctx, legacy.ID, 0, "scheduled", action))
	require.False(t, called)
	applyMonitoring(t, c, q)
	require.NoError(t, guard.Guard(ctx, legacy.ID, 0, "scheduled", action))
	no := false
	snapshotChange(t, snap, "status", legacy.ID, nil, &no)
	called = false
	require.Error(t, guard.Guard(ctx, legacy.ID, 0, "cleanup", action))
	require.False(t, called)
	_, e = snap.PolicyConfiguration(context.Background())
	require.Equal(t, "SNAPSHOT_ROOT_REQUIRED", dto.CodeOf(e))
}
func TestSnapshotPolicyNativePostgresConcurrentCreateAndReceipt(t *testing.T) {
	s, c, q, _, snap, _, _ := snapshotNative(t)
	bindAndAdoptSnapshot(t, s, snap)
	applyMonitoring(t, c, q)
	settings := snapshotSettings()
	in := snapshot.PolicyChange{RequestID: uuid.NewString(), ExpectedRevision: 1, Action: "save", Settings: &settings, Confirm: true}
	out, e := snap.ChangePolicyConfiguration(controlRoot(), in)
	require.NoError(t, e)
	again, e := snap.ChangePolicyConfiguration(controlRoot(), in)
	require.NoError(t, e)
	require.Equal(t, out.PolicyID, again.PolicyID)
	require.Equal(t, in.RequestID, out.PolicyID)
	applyMonitoring(t, c, q)
	start := make(chan struct{})
	results := make(chan error, 6)
	var wg sync.WaitGroup
	for n := 0; n < 6; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			v := settings
			v.Name = uuid.NewString()
			_, e := snap.ChangePolicyConfiguration(controlRoot(), snapshot.PolicyChange{RequestID: uuid.NewString(), ExpectedRevision: 2, Action: "save", Settings: &v, Confirm: true})
			results <- e
		}()
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
	conf, e := snap.PolicyConfiguration(controlRoot())
	require.NoError(t, e)
	require.Len(t, conf.Policies, 2)
	receipt, e := snap.PolicyConfigurationRequest(controlRoot(), in.RequestID)
	require.NoError(t, e)
	require.EqualValues(t, 2, receipt.OperationRevision)
	in.ExpectedPolicyRevision = 10
	_, e = snap.ChangePolicyConfiguration(controlRoot(), in)
	require.Equal(t, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT", dto.CodeOf(e))
}
func TestSnapshotPolicyNativePostgresTamperRollbackAndOldWorkerFence(t *testing.T) {
	s, c, q, db, snap, _, dir := snapshotNative(t)
	bindAndAdoptSnapshot(t, s, snap)
	applyMonitoring(t, c, q)
	p := snapshotSettings()
	out := snapshotChange(t, snap, "save", "", &p, nil)
	applyMonitoring(t, c, q)
	snapshotChange(t, snap, "current", out.PolicyID, nil, nil)
	applyMonitoring(t, c, q)
	cold := snapshot.NewService(context.Background(), snapshot.Config{Directory: dir, Instance: "owned-install"}, snapshot.Options{Coordinator: true})
	defer cold.Close()
	_, e := cold.SavePolicy(controlRoot(), snapshot.Policy{IntervalMinutes: 60, RetentionDays: 7, RetentionCount: 1})
	require.Equal(t, "SNAPSHOT_POLICY_CONTROL_REQUIRED", dto.CodeOf(e))
	_, e = cold.StartForPolicy(controlRoot(), uuid.NewString(), out.PolicyID)
	require.Equal(t, "SNAPSHOT_POLICY_CONTROL_UNAVAILABLE", dto.CodeOf(e))
	_, e = s.opts.Budget.Document(controlRoot(), "snapshot-policy-control", func([]byte) ([]byte, error) { return []byte(`{"version":1}`), nil })
	require.NoError(t, e)
	guard := &snapshotPolicyControl{s, snap}
	called := false
	require.Error(t, guard.Guard(controlRoot(), out.PolicyID, 0, "manual", func(snapshot.PolicyEntry) error { called = true; return nil }))
	require.False(t, called)
	require.NoError(t, os.Remove(filepath.Join(s.opts.Budget.ChangeDirectory(), "snapshot-policy-control.json")))
	require.NoError(t, db.Where("scope_id = ? AND domain = ?", c.opts.Scope.ID(), ControlSnapshotPolicies).Delete(&cm.ControlConfiguration{}).Error)
	_, e = snap.PolicyConfiguration(controlRoot())
	require.Equal(t, "SNAPSHOT_POLICY_RESTORE_CONFIRMATION_REQUIRED", dto.CodeOf(e))
}

func TestSnapshotPolicyNativePostgresCaptureRetentionPauseAndUnknownIntent(t *testing.T) {
	s, c, q, _, snap, backend, dir := snapshotNative(t)
	ctx := controlRoot()
	bindAndAdoptSnapshot(t, s, snap)
	applyMonitoring(t, c, q)
	settings := snapshotSettings()
	out := snapshotChange(t, snap, "save", "", &settings, nil)
	applyMonitoring(t, c, q)
	snapshotChange(t, snap, "current", out.PolicyID, nil, nil)
	applyMonitoring(t, c, q)
	capture := func() snapshot.Snapshot {
		key := uuid.NewString()
		var job snapshot.Snapshot
		var err error
		require.Eventually(t, func() bool { job, err = startControlledSnapshot(snap, ctx, key, out.PolicyID); return err == nil }, 3*time.Second, 10*time.Millisecond)
		require.Eventually(t, func() bool {
			job, err = snap.Get(ctx, job.ID)
			return err == nil && (job.Status == "ready" || job.Status == "failed")
		}, 3*time.Second, 10*time.Millisecond)
		require.Equal(t, "ready", job.Status, job.Error)
		require.Eventually(t, func() bool { _, e := snap.Protect(ctx, job.ID, false); return e == nil }, 3*time.Second, 10*time.Millisecond)
		job, err = snap.Get(ctx, job.ID)
		require.NoError(t, err)
		require.Positive(t, job.PolicyRevision)
		return job
	}
	first := capture()
	_, e := snap.Protect(ctx, first.ID, true)
	require.NoError(t, e)
	yes := true
	snapshotChange(t, snap, "status", out.PolicyID, nil, &yes)
	applyMonitoring(t, c, q)
	second := capture()
	third := capture()
	deleted, e := snap.Get(ctx, second.ID)
	require.NoError(t, e)
	require.Equal(t, "pruned", deleted.Status)
	require.NotEmpty(t, deleted.RetentionToken)
	protected, e := snap.Get(ctx, first.ID)
	require.NoError(t, e)
	require.Equal(t, "ready", protected.Status)
	started, release := make(chan struct{}), make(chan struct{})
	backend.mu.Lock()
	backend.wait = func() { close(started); <-release }
	backend.mu.Unlock()
	fourth, e := startControlledSnapshot(snap, ctx, uuid.NewString(), out.PolicyID)
	require.NoError(t, e)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("capture never reached fixture")
	}
	no := false
	snapshotChange(t, snap, "status", out.PolicyID, nil, &no)
	close(release)
	backend.mu.Lock()
	backend.wait = nil
	backend.mu.Unlock()
	require.Eventually(t, func() bool { j, e := snap.Get(ctx, fourth.ID); return e == nil && j.CleanupError != "" }, 3*time.Second, 10*time.Millisecond)
	still, e := snap.Get(ctx, third.ID)
	require.NoError(t, e)
	require.Equal(t, "ready", still.Status)
	applyMonitoring(t, c, q)
	// 模拟进程在隔离目录后中断：后续自动保留不重做，明确人工删除可清除本次隔离与记录。
	token := uuid.NewString()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "retention"), 0700))
	require.NoError(t, os.Rename(filepath.Join(dir, "contents", third.ID), filepath.Join(dir, "retention", token)))
	still.RetentionToken = token
	still.Status = "prune_unconfirmed"
	still.Phase = still.Status
	raw, _ := json.Marshal(still)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "snapshots", third.ID+".json"), raw, 0600))
	next := capture()
	require.NotEmpty(t, next.CleanupError)
	_, e = os.Stat(filepath.Join(dir, "retention", token))
	require.NoError(t, e)
	result, e := snap.Delete(ctx, third.ID, third.ID, false)
	require.NoError(t, e)
	require.True(t, result.FilesDeleted)
	_, e = os.Stat(filepath.Join(dir, "retention", token))
	require.True(t, os.IsNotExist(e))
}

func TestSnapshotPolicyNativePostgresLegacyDriftBeforePublication(t *testing.T) {
	s, c, q, _, snap, _, dir := snapshotNative(t)
	_, e := snap.PutPolicy(controlRoot(), "", "legacy", snapshot.Policy{IntervalMinutes: 60, RetentionDays: 7, RetentionCount: 1})
	require.NoError(t, e)
	bindAndAdoptSnapshot(t, s, snap)
	file := filepath.Join(dir, "policies.json")
	raw, e := os.ReadFile(file)
	require.NoError(t, e)
	var d map[string]any
	require.NoError(t, json.Unmarshal(raw, &d))
	d["policies"].([]any)[0].(map[string]any)["name"] = "out of band change"
	raw, e = json.Marshal(d)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(file, raw, 0600))
	items, e := q.Dequeue(context.Background(), controlQueueTenant, c.subscriber(), "owned-snapshot", "owned-worker", 1, time.Millisecond)
	require.NoError(t, e)
	require.Len(t, items, 1)
	require.Error(t, c.ApplyDelivery(context.Background(), items[0]))
	binding, e := snap.ReadPolicyBinding()
	require.NoError(t, e)
	require.Zero(t, binding.Revision)
	conf, e := snap.PolicyConfiguration(controlRoot())
	require.NoError(t, e)
	require.Equal(t, "failed", conf.State)
	require.Zero(t, conf.AppliedRevision)
}

func startControlledSnapshot(snap *snapshot.Service, ctx context.Context, key, id string) (snapshot.Snapshot, error) {
	o, e := snap.Overview(ctx)
	if e != nil {
		return snapshot.Snapshot{}, e
	}
	var rev int64
	for _, p := range o.Policies {
		if p.ID == id {
			rev = p.Revision
		}
	}
	return snap.StartForPolicyRevision(ctx, key, id, rev)
}
