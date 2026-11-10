package storagegov

import (
	"context"
	"errors"
	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	lifecycle "github.com/ArtisanCloud/PowerX/internal/infra/storage_lifecycle"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRuntimeLifecycleTickIsIndependentOfObservationAndWallClock(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	now := time.Now()
	for name, age := range map[string]time.Duration{"app.log": 30 * 24 * time.Hour, "app.log.1": 24 * time.Hour, "app.log.2": 8 * 24 * time.Hour} {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte("scheduler acceptance"), 0600))
		require.NoError(t, os.Chtimes(path, now.Add(-age), now.Add(-age)))
	}
	c, err := budget.New(budget.Config{Directory: filepath.Join(t.TempDir(), "state"), FilesystemPath: dir, HostID: "host", FilesystemID: "fs", InstanceID: "dev", MinFreeBytes: 1 << 20, CriticalFreeBytes: 1})
	require.NoError(t, err)
	e, err := lifecycle.New(lifecycle.Options{Coordinator: c, InstanceID: "dev", AllowEnforce: true, Sources: []lifecycle.Source{{ID: "logs", Name: "Logs", Kind: "application_logs", Scope: "dev", InstanceID: "dev", Directory: dir, Format: "numeric", ActiveFiles: []string{"app.log"}, OwnerUID: uint32(os.Geteuid())}}})
	require.NoError(t, err)
	_, err = e.Save(context.Background(), 1, "logs", 0, lifecycle.Policy{Enabled: true, RetentionDays: 7, MaxBytes: 1 << 20})
	require.NoError(t, err)
	s, err := NewService(Options{FilesystemPath: dir, Collector: runtimeUnknownCollector{}, Budget: c, Lifecycle: e, Now: func() time.Time { return now }, Identity: Identity{HostID: "host", FilesystemID: "fs", InstanceID: "dev"}})
	require.NoError(t, err)
	events, unsubscribe := s.SubscribeRuntime()
	defer unsubscribe()
	observation, cleanup := make(chan time.Time), make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done, failures := make(chan struct{}), make(chan error, 8)
	go func() {
		defer close(done)
		s.runRuntimeSchedule(ctx, observation, cleanup, func(err error) { failures <- err })
	}()
	// Even when observation and cleanup have the same clock value, cleanup must run.
	select {
	case observation <- now:
	case <-time.After(5 * time.Second):
		t.Fatal("observation timer was not consumed")
	}
	select {
	case event := <-events:
		require.Equal(t, "capacity_observed", event.Reason)
	case <-time.After(5 * time.Second):
		t.Fatal("observation did not finish")
	}
	select {
	case cleanup <- now:
	case <-time.After(5 * time.Second):
		t.Fatal("cleanup timer was not consumed")
	}
	require.Eventually(t, func() bool { _, err := os.Stat(filepath.Join(dir, "app.log.2")); return os.IsNotExist(err) }, 5*time.Second, 10*time.Millisecond)
	require.FileExists(t, filepath.Join(dir, "app.log"))
	require.FileExists(t, filepath.Join(dir, "app.log.1"))
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("scheduler did not stop")
	}
	require.Empty(t, failures)
}

func TestCapacityNotificationRetriesSameIdentityAndRecoveryIsSeparate(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	c, err := budget.New(budget.Config{Directory: filepath.Join(t.TempDir(), "state"), FilesystemPath: t.TempDir(), HostID: "host", FilesystemID: "fs", InstanceID: "dev", MinFreeBytes: 1 << 20, CriticalFreeBytes: 1})
	require.NoError(t, err)
	// An unknown collector result is an actionable diagnostic, never zero usage.
	calls := []CapacityNotification{}
	fail := true
	s, err := NewService(Options{FilesystemPath: "/", Collector: runtimeUnknownCollector{}, Budget: c, Now: clock, Identity: Identity{HostID: "host", FilesystemID: "fs", InstanceID: "dev"}, Notify: func(_ context.Context, n CapacityNotification) error {
		calls = append(calls, n)
		if fail {
			return errors.New("fixture unavailable")
		}
		return nil
	}})
	require.NoError(t, err)
	require.NoError(t, s.observeCapacity(context.Background()))
	require.Len(t, calls, 1)
	require.NoError(t, s.observeCapacity(context.Background()))
	require.Len(t, calls, 1)
	now = now.Add(5 * time.Minute)
	fail = false
	require.NoError(t, s.observeCapacity(context.Background()))
	require.Len(t, calls, 2)
	require.Equal(t, calls[0].ID, calls[1].ID)
	state := s.runtimeSnapshot(context.Background())
	require.NotNil(t, state.LastNotificationAt)
	require.Empty(t, state.NotificationProblem)
	now = now.Add(5 * time.Minute)
	require.NoError(t, s.observeCapacity(context.Background()))
	require.Len(t, calls, 2)
}

type runtimeUnknownCollector struct{}

func (runtimeUnknownCollector) Collect(context.Context, string, []Resource) Snapshot {
	return Snapshot{Filesystem: Filesystem{State: "unavailable"}}
}
