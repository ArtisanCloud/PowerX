package storagegov

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	lifecycle "github.com/ArtisanCloud/PowerX/internal/infra/storage_lifecycle"
	taskqueue "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func lifecycleNative(t *testing.T) (*Service, *ControlService, *taskqueue.StreamDriver, *gorm.DB, string) {
	t.Helper()
	db, q := ownedControlPostgres(t)
	dir, e := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, e)
	c, e := budget.New(budget.Config{Directory: filepath.Join(t.TempDir(), "runtime"), FilesystemPath: dir, HostID: "owned-host", FilesystemID: "owned-fs", InstanceID: "owned-install", MinFreeBytes: 1 << 20, CriticalFreeBytes: 1})
	require.NoError(t, e)
	for _, name := range []string{"app.log", "app.log.1.gz", "app.log.2.gz", "app.log.3.gz", "invoice.txt"} {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte("owned file content"), 0600))
		age := time.Duration(9*24) * time.Hour
		if name == "app.log" {
			age = 0
		}
		if name == "app.log.1.gz" {
			age = 24 * time.Hour
		}
		at := time.Now().Add(-age)
		require.NoError(t, os.Chtimes(path, at, at))
	}
	second := filepath.Join(dir, "second")
	require.NoError(t, os.Mkdir(second, 0700))
	engine, e := lifecycle.New(lifecycle.Options{Coordinator: c, InstanceID: "owned-install", AllowEnforce: true, Sources: []lifecycle.Source{
		{ID: "logs", Name: "Owned logs", Kind: "application_logs", Scope: "test", InstanceID: "owned-install", Directory: dir, Format: "numeric", ActiveFiles: []string{"app.log"}, OwnerUID: uint32(os.Geteuid())},
		{ID: "second", Name: "Second logs", Kind: "application_logs", Scope: "test", InstanceID: "owned-install", Directory: second, Format: "numeric", ActiveFiles: []string{"app.log"}, OwnerUID: uint32(os.Geteuid())},
	}})
	require.NoError(t, e)
	_, e = engine.Save(context.Background(), 1, "logs", 0, lifecycle.Policy{Enabled: true, RetentionDays: 7, MaxBytes: 1 << 20, CleanupMode: "expire_delete"})
	require.NoError(t, e)
	var svc *Service
	control, e := NewControlService(db, ControlOptions{Scope: ControlScope{"owned-host", "owned-fs", "owned-install"}, Queue: q, Changed: func() {
		if svc != nil {
			svc.PublishChange("configuration_changed")
		}
	}})
	require.NoError(t, e)
	svc, e = NewService(Options{Identity: Identity{HostID: "owned-host", FilesystemID: "owned-fs", InstanceID: "owned-install", Environment: "test"}, FilesystemPath: dir, Collector: &fixtureCollector{}, DB: db, Budget: c, Control: control, ControlWorkerEnabled: true, Lifecycle: engine})
	require.NoError(t, e)
	return svc, control, q, db, dir
}
func adoptNativeLifecycle(t *testing.T, s *Service, c *ControlService, q *taskqueue.StreamDriver) LifecycleSettings {
	t.Helper()
	out, e := s.LifecycleConfiguration(controlRoot(), "logs")
	require.NoError(t, e)
	saved, e := s.AdoptLifecycle(controlRoot(), "logs", out.LegacyRevision, out.LegacyDigest, uuid.NewString())
	require.NoError(t, e)
	require.False(t, saved.OperationPolicy.Enabled)
	applyMonitoring(t, c, q)
	return saved.OperationPolicy
}
func TestLifecycleNativePostgresAdoptionPublicationDeletionAndReceipts(t *testing.T) {
	s, c, q, db, dir := lifecycleNative(t)
	ctx := controlRoot()
	before, e := s.opts.Budget.Document(ctx, "lifecycle", nil)
	require.NoError(t, e)
	old, e := s.LifecycleConfiguration(ctx, "logs")
	require.NoError(t, e)
	require.Equal(t, "legacy", old.Authority)
	require.True(t, old.LegacyEnabled)
	_, e = s.SaveRuntimePolicy(ctx, "logs", 1, lifecycle.Policy{})
	require.Equal(t, "STORAGE_CONTROL_PRECONDITION_REQUIRED", dto.CodeOf(e))
	p := adoptNativeLifecycle(t, s, c, q)
	_, e = s.opts.Lifecycle.RunWithKey(ctx, "logs", "paused")
	require.Error(t, e)
	require.FileExists(t, filepath.Join(dir, "app.log.2.gz"))
	p.Enabled = true
	key := uuid.NewString()
	saved, e := s.SaveLifecycleConfiguration(ctx, "logs", 1, key, p)
	require.NoError(t, e)
	require.EqualValues(t, 2, saved.OperationRevision)
	require.Equal(t, "pending", saved.Configuration.State)
	_, e = s.opts.Lifecycle.RunWithKey(ctx, "logs", "pending")
	require.Error(t, e)
	applyMonitoring(t, c, q)
	out, e := s.LifecycleConfiguration(ctx, "logs")
	require.NoError(t, e)
	require.Equal(t, "applied", out.State)
	require.EqualValues(t, 2, *out.SnapshotRevision)
	run, e := s.opts.Lifecycle.RunWithRevision(ctx, "logs", "owned-real-delete", 2)
	require.NoError(t, e)
	require.Equal(t, "completed", run.State)
	require.Equal(t, 2, run.DeletedFiles)
	for _, name := range []string{"app.log", "app.log.1.gz", "invoice.txt"} {
		require.FileExists(t, filepath.Join(dir, name))
	}
	for _, name := range []string{"app.log.2.gz", "app.log.3.gz"} {
		require.NoFileExists(t, filepath.Join(dir, name))
	}
	replay, e := s.opts.Lifecycle.RunWithRevision(ctx, "logs", "owned-real-delete", 2)
	require.NoError(t, e)
	require.Equal(t, run.ID, replay.ID)
	again, e := s.SaveLifecycleConfiguration(ctx, "logs", 1, key, p)
	require.NoError(t, e)
	require.EqualValues(t, 2, again.OperationRevision)
	receipt, e := s.LifecycleRequest(ctx, "logs", key)
	require.NoError(t, e)
	require.Equal(t, p, receipt.OperationPolicy)
	second, e := s.LifecycleConfiguration(ctx, "second")
	require.NoError(t, e)
	_, e = s.AdoptLifecycle(ctx, "second", second.LegacyRevision, second.LegacyDigest, uuid.NewString())
	require.NoError(t, e)
	p.Enabled = false
	_, e = s.SaveLifecycleConfiguration(ctx, "logs", 2, uuid.NewString(), p)
	require.NoError(t, e)
	status, e := s.opts.Lifecycle.Status(ctx)
	require.NoError(t, e)
	for _, x := range status.Policies {
		if x.SourceID == "logs" {
			require.False(t, x.Enabled)
		}
	}
	source, _ := s.opts.Lifecycle.Source("logs")
	called := false
	_, pub, e := s.opts.Lifecycle.PolicySnapshot(ctx, "logs")
	require.NoError(t, e)
	require.Error(t, s.fenceLifecycle(ctx, source, pub.Policy, func() error { called = true; return nil }))
	require.False(t, called)
	applyMonitoring(t, c, q)
	var count int64
	require.NoError(t, db.Model(&m.ControlRevision{}).Where("domain = ?", string(lifecycleDomain("logs"))).Count(&count).Error)
	require.EqualValues(t, 3, count)
	after, e := s.opts.Budget.Document(ctx, "lifecycle", nil)
	require.NoError(t, e)
	var a, b struct {
		Policies json.RawMessage `json:"policies"`
	}
	require.NoError(t, json.Unmarshal(before, &a))
	require.NoError(t, json.Unmarshal(after, &b))
	require.JSONEq(t, string(a.Policies), string(b.Policies))
}
func TestLifecycleNativePostgresPauseAndFirstAdoptionFence(t *testing.T) {
	for _, managed := range []bool{false, true} {
		t.Run(map[bool]string{false: "first-adoption", true: "pause"}[managed], func(t *testing.T) {
			s, c, q, _, _ := lifecycleNative(t)
			ctx := controlRoot()
			var settings LifecycleSettings
			if managed {
				settings = adoptNativeLifecycle(t, s, c, q)
				settings.Enabled = true
				_, e := s.SaveLifecycleConfiguration(ctx, "logs", 1, uuid.NewString(), settings)
				require.NoError(t, e)
				applyMonitoring(t, c, q)
			}
			legacy, pub, e := s.opts.Lifecycle.PolicySnapshot(ctx, "logs")
			require.NoError(t, e)
			p := legacy
			if pub != nil {
				p = pub.Policy
			}
			source, _ := s.opts.Lifecycle.Source("logs")
			entered, release := make(chan struct{}), make(chan struct{})
			done := make(chan error, 1)
			go func() {
				done <- s.fenceLifecycle(ctx, source, p, func() error { close(entered); <-release; return nil })
			}()
			<-entered
			write := make(chan error, 1)
			go func() {
				if managed {
					settings.Enabled = false
					_, err := s.SaveLifecycleConfiguration(ctx, "logs", 2, uuid.NewString(), settings)
					write <- err
				} else {
					cfg, err := s.LifecycleConfiguration(ctx, "logs")
					if err == nil {
						_, err = s.AdoptLifecycle(ctx, "logs", cfg.LegacyRevision, cfg.LegacyDigest, uuid.NewString())
					}
					write <- err
				}
			}()
			select {
			case err := <-write:
				t.Fatalf("configuration overtook bounded file commit: %v", err)
			case <-time.After(120 * time.Millisecond):
			}
			close(release)
			require.NoError(t, <-done)
			require.NoError(t, <-write)
			require.Error(t, s.fenceLifecycle(ctx, source, p, func() error { t.Fatal("old revision executed after acknowledged stop"); return nil }))
		})
	}
}
func TestLifecycleNativePostgresPublisherRejectsChangedRegisteredSource(t *testing.T) {
	s, c, _, _, _ := lifecycleNative(t)
	ctx := controlRoot()
	cfg, e := s.LifecycleConfiguration(ctx, "logs")
	require.NoError(t, e)
	_, e = s.AdoptLifecycle(ctx, "logs", cfg.LegacyRevision, cfg.LegacyDigest, uuid.NewString())
	require.NoError(t, e)
	state, e := c.read(ctx, lifecycleDomain("logs"))
	require.NoError(t, e)
	d, e := decodeLifecycleDocument(state.Document)
	require.NoError(t, e)
	d.Fingerprint = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	raw, e := json.Marshal(d)
	require.NoError(t, e)
	e = c.bindings[lifecycleDomain("logs")].Apply(ctx, ControlPublication{Scope: c.opts.Scope, Revision: 1, Document: raw, ActorID: 1, UpdatedAt: time.Now().UTC()})
	require.ErrorIs(t, e, lifecycle.ErrBlocked)
	_, pub, e := s.opts.Lifecycle.PolicySnapshot(ctx, "logs")
	require.NoError(t, e)
	require.Nil(t, pub)
}

func TestLifecycleNativePostgresMissingDatabaseAndTamperedSnapshotBlock(t *testing.T) {
	s, c, q, db, dir := lifecycleNative(t)
	ctx := controlRoot()
	p := adoptNativeLifecycle(t, s, c, q)
	p.Enabled = true
	_, e := s.SaveLifecycleConfiguration(ctx, "logs", 1, uuid.NewString(), p)
	require.NoError(t, e)
	applyMonitoring(t, c, q)
	_, e = s.opts.Budget.Document(ctx, "lifecycle", func(raw []byte) ([]byte, error) {
		var d map[string]any
		require.NoError(t, json.Unmarshal(raw, &d))
		pub := d["published"].(map[string]any)["logs"].(map[string]any)
		pub["instance_id"] = "other-install"
		return json.Marshal(d)
	})
	require.NoError(t, e)
	_, e = s.opts.Lifecycle.RunWithKey(ctx, "logs", "tamper")
	require.Error(t, e)
	cfg, e := s.LifecycleConfiguration(ctx, "logs")
	require.NoError(t, e)
	require.Equal(t, "unconfirmed", cfg.State)
	require.NoError(t, db.Migrator().DropTable(&m.ControlConfiguration{}))
	_, e = s.opts.Lifecycle.RunWithKey(ctx, "logs", "no-db")
	require.Error(t, e)
	require.FileExists(t, filepath.Join(dir, "app.log.2.gz"))
}
