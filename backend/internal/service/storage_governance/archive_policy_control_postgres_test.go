package storagegov

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	archive "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func policyNative(t *testing.T) (*archivePolicyControl, *ControlService, archive.Policy, string) {
	t.Helper()
	s, ctrl, _, _ := monitoringNative(t)
	dir := t.TempDir()
	source := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(source, "app.log"), []byte("owned active"), 0600))
	a := archive.NewService(context.Background(), archive.Config{Directory: dir, Instance: "owned", ExecutionNodeID: "owned", Sources: []archive.Source{{ID: "logs", Name: "owned", Kind: "application_logs", Directory: source, RotationFormat: "numeric", ActiveFiles: []string{"app.log"}}}}, archive.Options{})
	t.Cleanup(a.Close)
	p := archive.Policy{ID: uuid.NewString(), Revision: 7, Name: "旧启用策略", SourceID: "logs", IntervalMinutes: 5, RetentionDays: 7, RetentionCount: 168, RemoteRetentionDays: 30, RemoteRetentionCount: 100, RemoteMaxBytes: 1 << 30, Enabled: true, RemoteCleanupEnabled: true, CreatedAt: time.Now().UTC()}
	raw, e := json.Marshal(p)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "policies", p.ID+".json"), raw, 0600))
	require.NoError(t, s.BindArchiveControl(a))
	return &archivePolicyControl{s, a}, ctrl, p, dir
}
func adoptNativePolicy(t *testing.T, c *archivePolicyControl, ctrl *ControlService) {
	t.Helper()
	out, e := c.Configuration(controlRoot())
	require.NoError(t, e)
	_, e = c.Adopt(controlRoot(), archive.PolicyAdoption{RequestID: uuid.NewString(), LegacyDigest: out.LegacyDigest})
	require.NoError(t, e)
	applyMonitoring(t, ctrl, ctrl.opts.Queue)
}
func TestArchivePolicyNativePostgresAdoptionDraftCASAndImmutableReceipts(t *testing.T) {
	c, ctrl, p, dir := policyNative(t)
	ctx := controlRoot()
	path := filepath.Join(dir, "policies", p.ID+".json")
	before, e := os.ReadFile(path)
	require.NoError(t, e)
	out, e := c.Configuration(ctx)
	require.NoError(t, e)
	require.Equal(t, "legacy", out.Authority)
	require.False(t, out.Policies[0].Enabled)
	_, e = c.archives.SavePolicy(ctx, p)
	require.Equal(t, "ARCHIVE_POLICY_CONTROL_REQUIRED", dto.CodeOf(e))
	key := uuid.NewString()
	adoption := archive.PolicyAdoption{RequestID: key, LegacyDigest: out.LegacyDigest}
	saved, e := c.Adopt(ctx, adoption)
	require.NoError(t, e)
	require.EqualValues(t, 1, saved.OperationRevision)
	require.False(t, saved.Configuration.Policies[0].Enabled)
	require.False(t, saved.Configuration.Policies[0].RemoteCleanupEnabled)
	_, e = c.Resolve(ctx, p.ID)
	require.Equal(t, "ARCHIVE_POLICY_PUBLICATION_PENDING", dto.CodeOf(e))
	applyMonitoring(t, ctrl, ctrl.opts.Queue)
	active, e := c.Resolve(ctx, p.ID)
	require.NoError(t, e)
	require.EqualValues(t, 8, active.Revision)
	settings := archive.SettingsOfPolicy(p)
	settings.Name = "已编辑草稿"
	in := archive.PolicyChange{RequestID: uuid.NewString(), ExpectedRevision: 1, ExpectedPolicyRevision: 8, PolicyID: p.ID, Action: "save", Settings: &settings}
	saved, e = c.Save(ctx, in)
	require.NoError(t, e)
	require.EqualValues(t, 2, saved.OperationRevision)
	_, e = c.Resolve(ctx, p.ID)
	require.Equal(t, "ARCHIVE_POLICY_PUBLICATION_PENDING", dto.CodeOf(e))
	replay, e := c.Save(ctx, in)
	require.NoError(t, e)
	require.EqualValues(t, 2, replay.OperationRevision)
	conflict := in
	changed := settings
	changed.RetentionDays = 1
	conflict.Settings = &changed
	_, e = c.Save(ctx, conflict)
	require.Equal(t, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT", dto.CodeOf(e))
	receipt, e := c.Request(ctx, key)
	require.NoError(t, e)
	require.EqualValues(t, 1, receipt.OperationRevision)
	require.EqualValues(t, 2, receipt.Configuration.DesiredRevision)
	applyMonitoring(t, ctrl, ctrl.opts.Queue)
	active, e = c.Resolve(ctx, p.ID)
	require.NoError(t, e)
	require.False(t, active.Enabled)
	require.False(t, active.RemoteCleanupEnabled)
	final, e := os.ReadFile(path)
	require.NoError(t, e)
	require.Equal(t, before, final)
	var count int64
	require.NoError(t, ctrl.repo.DB.Model(&m.ControlRevision{}).Where("domain = ?", string(ControlArchivePolicies)).Count(&count).Error)
	require.EqualValues(t, 2, count)
}
func TestArchivePolicyNativePostgresExplicitStatusAndPausedGuard(t *testing.T) {
	c, ctrl, p, _ := policyNative(t)
	ctx := controlRoot()
	adoptNativePolicy(t, c, ctrl)
	yes, no := true, false
	in := archive.PolicyChange{RequestID: uuid.NewString(), ExpectedRevision: 1, ExpectedPolicyRevision: 8, PolicyID: p.ID, Action: "status", Enabled: &yes, RemoteCleanupEnabled: &no}
	_, e := c.Save(ctx, in)
	require.Equal(t, "ARCHIVE_POLICY_CONFIRMATION_REQUIRED", dto.CodeOf(e))
	in.Confirm = true
	_, e = c.Save(ctx, in)
	require.Error(t, e, "cannot enable without a verified target")
	in.Enabled = &no
	saved, e := c.Save(ctx, in)
	require.NoError(t, e)
	require.EqualValues(t, 2, saved.OperationRevision)
	called := false
	active := saved.Configuration.Policies[0]
	e = c.Guard(ctx, active, "remote-cleanup", func([]archive.Policy, *archive.Target) error { called = true; return nil })
	require.Error(t, e)
	require.False(t, called)
	applyMonitoring(t, ctrl, ctrl.opts.Queue)
	active, e = c.Resolve(ctx, p.ID)
	require.NoError(t, e)
	e = c.Guard(ctx, active, "remote-cleanup", func([]archive.Policy, *archive.Target) error { called = true; return nil })
	require.Equal(t, "ARCHIVE_POLICY_DISABLED", dto.CodeOf(e))
	require.False(t, called)
	_, e = c.Request(ctx, uuid.NewString())
	require.Equal(t, "STORAGE_CONTROL_REQUEST_NOT_FOUND", dto.CodeOf(e))
}
func TestArchivePolicyNativePostgresGuardSerializesSaveWithSingleConnection(t *testing.T) {
	c, ctrl, p, _ := policyNative(t)
	ctx := controlRoot()
	adoptNativePolicy(t, c, ctrl)
	pool, e := ctrl.repo.DB.DB()
	require.NoError(t, e)
	for _, connections := range []int{1, 2} {
		pool.SetMaxOpenConns(connections)
		current, e := c.Configuration(ctx)
		require.NoError(t, e)
		policy := current.Policies[0]
		entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
		go func() {
			done <- c.Guard(ctx, archive.Policy{}, "local-retention", func([]archive.Policy, *archive.Target) error { close(entered); <-release; return nil })
		}()
		<-entered
		settings := archive.SettingsOfPolicy(p)
		saved := make(chan error, 1)
		go func() {
			_, e := c.Save(ctx, archive.PolicyChange{RequestID: uuid.NewString(), ExpectedRevision: current.DesiredRevision, ExpectedPolicyRevision: policy.Revision, PolicyID: p.ID, Action: "save", Settings: &settings})
			saved <- e
		}()
		select {
		case <-saved:
			t.Fatal("configuration changed inside execution guard")
		case <-time.After(50 * time.Millisecond):
		}
		close(release)
		require.NoError(t, <-done)
		require.NoError(t, <-saved)
		applyMonitoring(t, ctrl, ctrl.opts.Queue)
	}
}
func TestArchivePolicyNativePostgresConcurrentCreationAndProjectionRollback(t *testing.T) {
	c, ctrl, p, _ := policyNative(t)
	ctx := controlRoot()
	adoptNativePolicy(t, c, ctrl)
	settings := archive.SettingsOfPolicy(p)
	inputs := []archive.PolicyChange{{RequestID: uuid.NewString(), ExpectedRevision: 1, Action: "save", Settings: &settings}, {RequestID: uuid.NewString(), ExpectedRevision: 1, Action: "save", Settings: &settings}}
	var wg sync.WaitGroup
	errors := make([]error, 2)
	for i := range inputs {
		wg.Add(1)
		go func(i int) { defer wg.Done(); _, errors[i] = c.Save(ctx, inputs[i]) }(i)
	}
	wg.Wait()
	success, conflict := 0, 0
	for _, e := range errors {
		if e == nil {
			success++
		} else {
			require.Equal(t, "STORAGE_CONTROL_REVISION_CONFLICT", dto.CodeOf(e))
			conflict++
		}
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, conflict)
	applyMonitoring(t, ctrl, ctrl.opts.Queue)
	_, e := c.service.opts.Budget.Document(ctx, "archive-policy-control", func([]byte) ([]byte, error) { return []byte(`{"version":1,"revision":999}`), nil })
	require.NoError(t, e)
	_, e = c.Resolve(ctx, p.ID)
	require.Equal(t, "ARCHIVE_POLICY_SNAPSHOT_UNCONFIRMED", dto.CodeOf(e))
}
func TestArchivePolicyNativePostgresSupersededAdoptionStillChecksOrigin(t *testing.T) {
	c, ctrl, p, dir := policyNative(t)
	ctx := controlRoot()
	out, e := c.Configuration(ctx)
	require.NoError(t, e)
	_, e = c.Adopt(ctx, archive.PolicyAdoption{RequestID: uuid.NewString(), LegacyDigest: out.LegacyDigest})
	require.NoError(t, e)
	settings := archive.SettingsOfPolicy(p)
	_, e = c.Save(ctx, archive.PolicyChange{RequestID: uuid.NewString(), ExpectedRevision: 1, ExpectedPolicyRevision: 8, PolicyID: p.ID, Action: "save", Settings: &settings})
	require.NoError(t, e)
	p.Name = "外部改动"
	raw, _ := json.Marshal(p)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "policies", p.ID+".json"), raw, 0600))
	items, e := ctrl.opts.Queue.Dequeue(ctx, controlQueueTenant, ctrl.subscriber(), "owned-policy", "owned", 10, time.Millisecond)
	require.NoError(t, e)
	failed := false
	for _, item := range items {
		if ctrl.ApplyDelivery(ctx, item) != nil {
			failed = true
		}
	}
	require.True(t, failed)
	state, e := ctrl.read(ctx, ControlArchivePolicies)
	require.NoError(t, e)
	require.Zero(t, state.AppliedRevision)
}

func TestArchivePolicyNativePostgresMediaLocalScheduleRemainsAvailable(t *testing.T) {
	s, ctrl, _, _ := monitoringNative(t)
	dir, media := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(media, "owned.txt"), []byte("owned media"), 0600))
	a := archive.NewService(context.Background(), archive.Config{Directory: dir, Instance: "owned", ExecutionNodeID: "owned", Sources: []archive.Source{{ID: "media", Name: "owned media", Kind: "media", Directory: media}}}, archive.Options{})
	t.Cleanup(a.Close)
	p, e := a.SavePolicy(controlRoot(), archive.Policy{Name: "本地媒体快照", SourceID: "media", IntervalMinutes: 60, RetentionDays: 7, RetentionCount: 1, RemoteRetentionDays: 30, RemoteRetentionCount: 100})
	require.NoError(t, e)
	require.NoError(t, s.BindArchiveControl(a))
	c := &archivePolicyControl{s, a}
	adoptNativePolicy(t, c, ctrl)
	yes, no := true, false
	in := archive.PolicyChange{RequestID: uuid.NewString(), ExpectedRevision: 1, ExpectedPolicyRevision: p.Revision + 1, PolicyID: p.ID, Action: "status", Enabled: &yes, RemoteCleanupEnabled: &no, Confirm: true}
	_, e = c.Save(controlRoot(), in)
	require.NoError(t, e)
	applyMonitoring(t, ctrl, ctrl.opts.Queue)
	caller, cancelCaller := context.WithCancel(controlRoot())
	job, e := a.Start(caller, p.ID, uuid.NewString(), "owned", "")
	cancelCaller()
	require.NoError(t, e)
	require.Eventually(t, func() bool {
		out, e := a.Job(controlRoot(), job.ID)
		return e == nil && out.Status == "success" && out.RemoteState == "not_requested"
	}, 3*time.Second, 20*time.Millisecond)
	require.NoError(t, os.WriteFile(filepath.Join(media, "owned.txt"), []byte("owned media v2"), 0600))
	nextKey := uuid.NewString()
	var next archive.Job
	require.Eventually(t, func() bool {
		caller, cancel := context.WithCancel(controlRoot())
		defer cancel()
		var err error
		next, err = a.Start(caller, p.ID, nextKey, "owned", "")
		return err == nil
	}, 3*time.Second, 20*time.Millisecond)
	require.Eventually(t, func() bool {
		old, e := a.Job(controlRoot(), job.ID)
		current, e2 := a.Job(controlRoot(), next.ID)
		return e == nil && e2 == nil && old.Status == "pruned" && current.Status == "success"
	}, 3*time.Second, 20*time.Millisecond)
	old, e := a.Job(controlRoot(), job.ID)
	require.NoError(t, e)
	require.NotEmpty(t, old.Files)
	require.Eventually(t, func() bool {
		_, err := os.Stat(filepath.Join(dir, "objects", old.Files[0].SHA256[:2], old.Files[0].SHA256))
		return os.IsNotExist(err)
	}, 3*time.Second, 20*time.Millisecond, "cleanup must outlive the original HTTP/Tick context")
	in.RequestID = uuid.NewString()
	in.ExpectedRevision = 2
	in.ExpectedPolicyRevision++
	in.Enabled = &no
	_, e = c.Save(controlRoot(), in)
	require.NoError(t, e)
	_, e = a.Start(controlRoot(), p.ID, uuid.NewString(), "owned", "")
	require.Error(t, e, "pending pause must immediately block a new scheduled run")
	applyMonitoring(t, ctrl, ctrl.opts.Queue)
	_, e = a.Start(controlRoot(), p.ID, uuid.NewString(), "owned", "")
	require.Equal(t, "ARCHIVE_POLICY_DISABLED", dto.CodeOf(e))
	data, e := os.ReadFile(filepath.Join(media, "owned.txt"))
	require.NoError(t, e)
	require.Equal(t, "owned media v2", string(data))
}
