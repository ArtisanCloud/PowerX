package storagelifecycle

import (
	"context"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
	"time"
)

func TestAuthorityFenceStopsFollowingFilesAndPreservesRequestJournal(t *testing.T) {
	e, dir := engineFixture(t, false)
	calls := 0
	require.NoError(t, e.BindAuthority(Authority{Resolve: func(_ context.Context, _ Source, p Policy, _ *Publication) (Policy, error) { return p, nil }, Fence: func(_ context.Context, _ Source, _ Policy, commit func() error) error {
		calls++
		if calls > 1 {
			return ErrBlocked
		}
		return commit()
	}}))
	_, err := e.Save(context.Background(), 1, "logs", 1, Policy{})
	require.ErrorIs(t, err, ErrManaged)
	run, err := e.RunWithKey(context.Background(), "logs", "authority-stop")
	require.NoError(t, err)
	require.Equal(t, 1, run.DeletedFiles)
	require.Equal(t, 2, calls)
	require.Equal(t, "configuration_authority_blocked", run.Problem)
	require.FileExists(t, filepath.Join(dir, "app.log"))
	require.FileExists(t, filepath.Join(dir, "app.log.1.gz"))
	require.FileExists(t, filepath.Join(dir, "invoice.txt"))
	again, err := e.RunWithKey(context.Background(), "logs", "authority-stop")
	require.NoError(t, err)
	require.Equal(t, run.ID, again.ID)
	require.Equal(t, 2, calls)
}
func TestAuthorityFenceBlocksBeforeAnyFileChange(t *testing.T) {
	e, dir := engineFixture(t, false)
	require.NoError(t, e.BindAuthority(Authority{Resolve: func(_ context.Context, _ Source, p Policy, _ *Publication) (Policy, error) { return p, nil }, Fence: func(context.Context, Source, Policy, func() error) error { return ErrBlocked }}))
	run, err := e.RunWithKey(context.Background(), "logs", "authority-unavailable")
	require.NoError(t, err)
	require.Zero(t, run.DeletedFiles)
	require.Equal(t, "blocked", run.State)
	require.FileExists(t, filepath.Join(dir, "app.log.2.gz"))
	require.FileExists(t, filepath.Join(dir, "app.log.3.gz"))
}

func TestAuthorityCommitFailureAfterUnlinkIsUnconfirmedAndNeverReplayed(t *testing.T) {
	e, dir := engineFixture(t, false)
	calls := 0
	require.NoError(t, e.BindAuthority(Authority{Resolve: func(_ context.Context, _ Source, p Policy, _ *Publication) (Policy, error) { return p, nil }, Fence: func(_ context.Context, _ Source, _ Policy, commit func() error) error {
		calls++
		if err := commit(); err != nil {
			return err
		}
		return ErrBlocked
	}}))
	run, err := e.RunWithKey(context.Background(), "logs", "commit-unknown")
	require.ErrorIs(t, err, ErrBlocked)
	require.Equal(t, "unconfirmed", run.State)
	require.Equal(t, 1, run.DeletedFiles)
	require.Equal(t, 1, calls)
	require.FileExists(t, filepath.Join(dir, "app.log"))
	require.FileExists(t, filepath.Join(dir, "invoice.txt"))
	replay, _ := e.RunWithKey(context.Background(), "logs", "commit-unknown")
	require.Equal(t, run.ID, replay.ID)
	require.Equal(t, 1, calls)
}

func TestAuthoritySnapshotNeverOverwritesAnotherInstallationOrSource(t *testing.T) {
	e, _ := engineFixture(t, false)
	ctx := context.Background()
	p, _, err := e.PolicySnapshot(ctx, "logs")
	require.NoError(t, err)
	p.ConfigurationDigest = "approved-digest"
	snapshot := Publication{Version: 1, HostID: "host", FilesystemID: "disk", InstanceID: "dev", Digest: p.ConfigurationDigest, Policy: p}
	require.NoError(t, e.PublishPolicy(ctx, "logs", snapshot))
	foreign := snapshot
	foreign.HostID = "another-host"
	foreign.Policy.Revision++
	require.ErrorIs(t, e.PublishPolicy(ctx, "logs", foreign), ErrBlocked)
	_, stored, err := e.PolicySnapshot(ctx, "logs")
	require.NoError(t, err)
	require.Equal(t, "host", stored.HostID)
	require.Equal(t, p.Revision, stored.Policy.Revision)
}

func TestLifecycleContendedLockWaitNeverReentersMutation(t *testing.T) {
	e, _ := engineFixture(t, false)
	ctx := context.Background()
	entered, release := make(chan struct{}), make(chan struct{})
	held := make(chan error, 1)
	go func() {
		_, err := e.opts.Coordinator.Document(ctx, "lifecycle", func(raw []byte) ([]byte, error) { close(entered); <-release; return nil, nil })
		held <- err
	}()
	<-entered
	calls := 0
	done := make(chan error, 1)
	go func() { done <- e.update(ctx, func(*document) error { calls++; return nil }) }()
	select {
	case <-done:
		t.Fatal("did not wait for owned host lock")
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-held)
	require.NoError(t, <-done)
	require.Equal(t, 1, calls)
}
