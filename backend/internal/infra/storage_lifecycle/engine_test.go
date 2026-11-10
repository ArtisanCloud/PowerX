package storagelifecycle

import (
	"bytes"
	"context"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	"github.com/stretchr/testify/require"
)

func engineFixture(t *testing.T, archive bool) (*Engine, string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	c, err := budget.New(budget.Config{Directory: filepath.Join(t.TempDir(), "state"), FilesystemPath: dir, HostID: "host", FilesystemID: "disk", InstanceID: "dev", MinFreeBytes: 1 << 20, CriticalFreeBytes: 1})
	require.NoError(t, err)
	for _, item := range []struct {
		name string
		age  time.Duration
	}{{"app.log", 0}, {"app.log.1.gz", 24 * time.Hour}, {"app.log.2.gz", 8 * 24 * time.Hour}, {"app.log.3.gz", 9 * 24 * time.Hour}, {"invoice.txt", 30 * 24 * time.Hour}} {
		path := filepath.Join(dir, item.name)
		require.NoError(t, os.WriteFile(path, []byte("retained fixture content"), 0600))
		at := time.Now().Add(-item.age)
		require.NoError(t, os.Chtimes(path, at, at))
	}
	e, err := New(Options{Coordinator: c, InstanceID: "dev", AllowEnforce: true, Sources: []Source{{ID: "logs", Name: "Logs", Kind: "application_logs", Scope: "dev", InstanceID: "dev", Directory: dir, Format: "numeric", ActiveFiles: []string{"app.log"}, OwnerUID: uint32(os.Geteuid()), RequireArchive: archive}}})
	require.NoError(t, err)
	p, err := e.Save(context.Background(), 1, "logs", 0, Policy{Enabled: true, RetentionDays: 7, MaxBytes: 1 << 20})
	require.NoError(t, err)
	require.EqualValues(t, 1, p.Revision)
	return e, dir
}
func TestLifecycleSealedFilesAreDeletedOnceAndBusinessFilesRemain(t *testing.T) {
	e, dir := engineFixture(t, false)
	ctx := context.Background()
	preview, err := e.Preview(ctx, "logs")
	require.NoError(t, err)
	require.Len(t, preview.Candidates, 2)
	run, err := e.RunWithKey(ctx, "logs", "request-1")
	require.NoError(t, err)
	require.Equal(t, "completed", run.State)
	require.Equal(t, 2, run.DeletedFiles)
	require.NotNil(t, run.AvailableAfter)
	for _, name := range []string{"app.log", "app.log.1.gz", "invoice.txt"} {
		require.FileExists(t, filepath.Join(dir, name))
	}
	replayed, err := e.RunWithKey(ctx, "logs", "request-1")
	require.NoError(t, err)
	require.Equal(t, run.ID, replayed.ID)
	require.Equal(t, 2, replayed.DeletedFiles)
	state, err := e.Status(ctx)
	require.NoError(t, err)
	require.Len(t, state.Runs, 1)
	require.False(t, state.DatabaseCleanupEnabled)
}
func TestLifecycleArchiveGateCannotBeDisabledBySavedPolicy(t *testing.T) {
	e, dir := engineFixture(t, true)
	ctx := context.Background()
	state, err := e.Status(ctx)
	require.NoError(t, err)
	require.True(t, state.Policies[0].RequireArchive)
	run, err := e.RunWithKey(ctx, "logs", "request-archive")
	require.NoError(t, err)
	require.Equal(t, "blocked", run.State)
	require.Equal(t, "archive_verification_required", run.Problem)
	require.Zero(t, run.DeletedFiles)
	require.FileExists(t, filepath.Join(dir, "app.log.2.gz"))
}

func TestAuditSourcesRequireArchiveEvenInDevelopmentAndCallerOptsOut(t *testing.T) {
	for _, kind := range []string{"audit_logs", "postgresql_logs", "journal"} {
		t.Run(kind, func(t *testing.T) {
			base, dir := engineFixture(t, false)
			e, err := New(Options{Coordinator: base.opts.Coordinator, InstanceID: "dev", AllowEnforce: true, Sources: []Source{{ID: "audit", Name: "Audit", Kind: kind, Scope: "dev", InstanceID: "dev", Directory: dir, Format: "numeric", ActiveFiles: []string{"app.log"}, OwnerUID: uint32(os.Geteuid())}}})
			require.NoError(t, err)
			p, err := e.Save(context.Background(), 1, "audit", 0, Policy{Enabled: true, RetentionDays: 7, MaxBytes: 1 << 20, RequireArchive: false})
			require.NoError(t, err)
			require.True(t, p.RequireArchive)
			r, err := e.RunWithKey(context.Background(), "audit", "unarchived-audit")
			require.NoError(t, err)
			require.Zero(t, r.DeletedFiles)
			require.Equal(t, "archive_verification_required", r.Problem)
			require.FileExists(t, filepath.Join(dir, "app.log.2.gz"))
		})
	}
}
func TestLifecycleLegacyOwnerAndPolicyChangeStopExecution(t *testing.T) {
	e, dir := engineFixture(t, true)
	e.opts.LegacyBlocked = func(context.Context) (bool, error) { return true, nil }
	_, err := e.RunWithKey(context.Background(), "logs", "legacy-owner")
	require.ErrorIs(t, err, ErrBlocked)
	e.opts.LegacyBlocked = nil
	e.opts.VerifyArchive = func(ctx context.Context, _ Source, _ Candidate) (bool, error) {
		_, err := e.Save(ctx, 1, "logs", 1, Policy{Enabled: false, RetentionDays: 7, MaxBytes: 1 << 20})
		require.NoError(t, err)
		return true, nil
	}
	run, err := e.RunWithKey(context.Background(), "logs", "paused-during-verify")
	require.NoError(t, err)
	require.Equal(t, "policy_changed", run.Problem)
	require.Zero(t, run.DeletedFiles)
	require.FileExists(t, filepath.Join(dir, "app.log.2.gz"))
}
func TestLifecycleRechecksReplacedFileAndKeepsUncertainIntents(t *testing.T) {
	e, dir := engineFixture(t, true)
	e.opts.VerifyArchive = func(_ context.Context, _ Source, c Candidate) (bool, error) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, c.Name), []byte("changed"), 0600))
		return true, nil
	}
	run, err := e.RunWithKey(context.Background(), "logs", "changed-file")
	require.NoError(t, err)
	require.Equal(t, "blocked", run.State)
	require.Zero(t, run.DeletedFiles)
	require.NoError(t, e.update(context.Background(), func(d *document) error {
		d.Runs = append(d.Runs, Run{ID: "crash", SourceID: "logs", State: "executing", PendingName: "app.log.2.gz", StartedAt: time.Now().Add(-time.Hour)})
		return nil
	}))
	state, err := e.Status(context.Background())
	require.NoError(t, err)
	require.Equal(t, "unconfirmed", state.Runs[len(state.Runs)-1].State)
	_, err = e.RunWithKey(context.Background(), "logs", "after-crash")
	require.ErrorIs(t, err, ErrBlocked)
}
func TestLifecycleDirectoryReplacementAndSymlinksFailClosed(t *testing.T) {
	e, dir := engineFixture(t, false)
	outside := filepath.Join(t.TempDir(), "business")
	require.NoError(t, os.WriteFile(outside, []byte("business"), 0600))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "app.log.9.gz")))
	p, err := e.Preview(context.Background(), "logs")
	require.NoError(t, err)
	require.Positive(t, p.Protected["unsafe_file"])
	require.NoError(t, os.Rename(dir, dir+"-old"))
	t.Cleanup(func() { _ = os.RemoveAll(dir + "-old") })
	require.NoError(t, os.Mkdir(dir, 0700))
	_, err = e.Preview(context.Background(), "logs")
	require.ErrorIs(t, err, ErrBlocked)
	content, err := os.ReadFile(outside)
	require.NoError(t, err)
	require.Equal(t, "business", string(content))
}

func TestLifecycleCrashAcknowledgementPreservesUnknownAndRequiresPause(t *testing.T) {
	e, dir := engineFixture(t, false)
	ctx := context.Background()
	require.NoError(t, e.update(ctx, func(d *document) error {
		d.Runs = append(d.Runs, Run{ID: "interrupted", RequestKey: "old", SourceID: "logs", InstanceID: "dev", PolicyRevision: 1, State: "executing", StartedAt: time.Now().Add(-time.Hour), PendingName: "app.log.2.gz"})
		return nil
	}))
	_, err := e.AcknowledgeInterrupted(ctx, 1, "interrupted")
	require.ErrorIs(t, err, ErrBlocked)
	_, err = e.Save(ctx, 1, "logs", 1, Policy{RetentionDays: 7, MaxBytes: 1 << 20})
	require.NoError(t, err)
	unknown, err := e.AcknowledgeInterrupted(ctx, 1, "interrupted")
	require.NoError(t, err)
	require.Equal(t, "unconfirmed", unknown.State)
	require.NotNil(t, unknown.AcknowledgedAt)
	require.Nil(t, unknown.AvailableAfter)
	require.FileExists(t, filepath.Join(dir, "app.log.2.gz"))
	_, err = e.Save(ctx, 1, "logs", 2, Policy{Enabled: true, RetentionDays: 7, MaxBytes: 1 << 20})
	require.NoError(t, err)
	_, err = e.RunWithRevision(ctx, "logs", "stale-preview", 1)
	require.ErrorIs(t, err, ErrConflict)
	run, err := e.RunWithRevision(ctx, "logs", "new-preview", 3)
	require.NoError(t, err)
	require.Equal(t, 2, run.DeletedFiles)
	_, err = e.Save(ctx, 1, "logs", 3, Policy{RetentionDays: 7, MaxBytes: 1 << 20})
	require.NoError(t, err)
	replay, err := e.RunWithRevision(ctx, "logs", "new-preview", 3)
	require.NoError(t, err)
	require.Equal(t, run.ID, replay.ID)
}
func TestLifecyclePreservesLatestForEveryStream(t *testing.T) {
	e, dir := engineFixture(t, false)
	for _, item := range []struct {
		name string
		days int
	}{{"error.log.1", 10}, {"error.log.2", 11}} {
		p := filepath.Join(dir, item.name)
		require.NoError(t, os.WriteFile(p, []byte("error"), 0600))
		at := time.Now().AddDate(0, 0, -item.days)
		require.NoError(t, os.Chtimes(p, at, at))
	}
	source := e.sources["logs"]
	source.ActiveFiles = append(source.ActiveFiles, "error.log")
	plan, err := InspectSealedSource(context.Background(), source)
	require.NoError(t, err)
	require.Contains(t, plan.CandidateNames, "error.log.2")
	require.NotContains(t, plan.CandidateNames, "error.log.1")
	require.NotContains(t, plan.CandidateNames, "app.log.1.gz")
}

func TestLifecyclePauseStillWorksWhenSourceBecameUnsupported(t *testing.T) {
	e, _ := engineFixture(t, false)
	s := e.sources["logs"]
	s.Supported = false
	s.Problem = "source_unavailable"
	e.sources["logs"] = s
	p, err := e.Save(context.Background(), 1, "logs", 1, Policy{Enabled: false, RetentionDays: 7, MaxBytes: 1 << 20})
	require.NoError(t, err)
	require.False(t, p.Enabled)
	_, err = e.Save(context.Background(), 1, "logs", 2, Policy{Enabled: true, RetentionDays: 7, MaxBytes: 1 << 20})
	require.ErrorIs(t, err, ErrBlocked)
}

func TestLifecycleAtomicHoldDoesNotDeleteNewlyRotatedReplacement(t *testing.T) {
	e, dir := engineFixture(t, false)
	e.beforeHold = func() {
		e.beforeHold = nil
		require.NoError(t, os.Rename(filepath.Join(dir, "app.log.3.gz"), filepath.Join(dir, "native-old")))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "app.log.3.gz"), []byte("fresh rotation"), 0600))
	}
	r, err := e.RunWithKey(context.Background(), "logs", "rotated-at-unlink")
	require.NoError(t, err)
	require.Equal(t, "partial", r.State)
	require.Equal(t, 1, r.DeletedFiles)
	raw, err := os.ReadFile(filepath.Join(dir, "app.log.3.gz"))
	require.NoError(t, err)
	require.Equal(t, "fresh rotation", string(raw))
	require.FileExists(t, filepath.Join(dir, "native-old"))
}
func TestLifecycleHoldConflictPreservesBothFilesAndBlocksUntilManualReconciliation(t *testing.T) {
	e, dir := engineFixture(t, false)
	e.beforeHold = func() {
		e.beforeHold = nil
		require.NoError(t, os.Rename(filepath.Join(dir, "app.log.3.gz"), filepath.Join(dir, "native-old")))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "app.log.3.gz"), []byte("first new rotation"), 0600))
	}
	e.afterHold = func() {
		e.afterHold = nil
		require.NoError(t, os.WriteFile(filepath.Join(dir, "app.log.3.gz"), []byte("second new rotation"), 0600))
	}
	r, err := e.RunWithKey(context.Background(), "logs", "rotation-restore-conflict")
	require.NoError(t, err)
	require.Equal(t, "unconfirmed", r.State)
	require.Equal(t, "quarantine_requires_review", r.Problem)
	require.NotEmpty(t, r.PendingQuarantine)
	require.Zero(t, r.DeletedFiles)
	raw, err := os.ReadFile(filepath.Join(dir, "app.log.3.gz"))
	require.NoError(t, err)
	require.Equal(t, "second new rotation", string(raw))
	require.FileExists(t, filepath.Join(dir, "native-old"))
	require.NoError(t, e.opts.Coordinator.WithQuarantine(context.Background(), func(fd int) error {
		f, err := unix.Openat(fd, r.PendingQuarantine, unix.O_RDONLY|unix.O_NOFOLLOW, 0)
		require.NoError(t, err)
		file := os.NewFile(uintptr(f), "held-test-log")
		raw, err := io.ReadAll(file)
		file.Close()
		require.NoError(t, err)
		require.Equal(t, "first new rotation", string(raw))
		return nil
	}))
	e.opts.Now = func() time.Time { return time.Now().Add(3 * time.Minute) }
	_, err = e.Save(context.Background(), 1, "logs", 1, Policy{RetentionDays: 7, MaxBytes: 1 << 20})
	require.NoError(t, err)
	_, err = e.AcknowledgeInterrupted(context.Background(), 1, r.ID)
	require.ErrorIs(t, err, ErrBlocked)
	require.NoError(t, e.opts.Coordinator.WithQuarantine(context.Background(), func(fd int) error {
		sourceFD, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
		require.NoError(t, err)
		defer unix.Close(sourceFD)
		require.NoError(t, unix.Linkat(fd, r.PendingQuarantine, sourceFD, "operator-restored.log", 0))
		return unix.Unlinkat(fd, r.PendingQuarantine, 0)
	}))
	acknowledged, err := e.AcknowledgeInterrupted(context.Background(), 1, r.ID)
	require.NoError(t, err)
	require.Equal(t, "unconfirmed", acknowledged.State)
	require.FileExists(t, filepath.Join(dir, "operator-restored.log"))
}

func TestExplicitDirectExpiryOverridesLegacyArchiveDefaultWithoutDisablingWrites(t *testing.T) {
	e, dir := engineFixture(t, true)
	p, err := e.Save(context.Background(), 1, "logs", 1, Policy{CleanupMode: "expire_delete", Enabled: true, RetentionDays: 7, MaxBytes: 1 << 20})
	require.NoError(t, err)
	require.False(t, p.RequireArchive)
	run, err := e.RunWithKey(context.Background(), "logs", "explicit-expiry")
	require.NoError(t, err)
	require.Equal(t, "completed", run.State)
	require.Equal(t, 2, run.DeletedFiles)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.log"), []byte("new SQL/audit logging continues"), 0600))
	require.FileExists(t, filepath.Join(dir, "app.log.1.gz"))
	require.FileExists(t, filepath.Join(dir, "invoice.txt"))
	p, err = e.Save(context.Background(), 1, "logs", p.Revision, Policy{CleanupMode: "disabled", RetentionDays: 7, MaxBytes: 1 << 20})
	require.NoError(t, err)
	require.False(t, p.Enabled)
}
func TestLegacyPersistedModeMustBeExplicitlyAdopted(t *testing.T) {
	e, _ := engineFixture(t, false)
	_, err := e.opts.Coordinator.Document(context.Background(), "lifecycle", func(raw []byte) ([]byte, error) {
		return bytes.ReplaceAll(raw, []byte(`"cleanup_mode":"expire_delete"`), []byte(`"cleanup_mode":""`)), nil
	})
	require.NoError(t, err)
	state, err := e.Status(context.Background())
	require.NoError(t, err)
	require.False(t, state.Policies[0].Enabled)
	require.Equal(t, "disabled", state.Policies[0].CleanupMode)
}
