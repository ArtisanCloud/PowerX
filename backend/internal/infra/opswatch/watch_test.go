package opswatch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCommittedLedgerChangesWakeSubscriberAndCancellationClosesIt(t *testing.T) {
	dir := t.TempDir()
	sub, e := Subscribe(context.Background(), map[string][]string{"snapshots": {dir}}, nil)
	require.NoError(t, e)
	defer sub.Close()
	temp := filepath.Join(dir, ".snapshot-staging")
	require.NoError(t, os.WriteFile(temp, []byte(`{"status":"ready"}`), 0600))
	require.NoError(t, os.Rename(temp, filepath.Join(dir, "job.json")))
	select {
	case names := <-sub.Changes:
		require.Equal(t, []string{"snapshots"}, names)
	case <-time.After(2 * time.Second):
		t.Fatal("atomic ledger commit did not push an update")
	}
}

func TestProgressSubscriptionFiltersOtherJobsAndIgnoresTemporaryFiles(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	sub, e := Subscribe(ctx, map[string][]string{"job": {dir}}, func(path string) bool { return filepath.Base(path) == "own.json" })
	require.NoError(t, e)
	defer sub.Close()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "other.json"), []byte(`{}`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".snapshot-temp"), []byte(`{}`), 0600))
	select {
	case names := <-sub.Changes:
		t.Fatalf("unrelated update leaked: %v", names)
	case <-time.After(80 * time.Millisecond):
	}
	cancel()
	select {
	case _, ok := <-sub.Changes:
		require.False(t, ok)
	case <-time.After(time.Second):
		t.Fatal("cancel did not release watcher")
	}
}
