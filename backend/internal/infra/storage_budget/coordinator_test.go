package storagebudget

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func fixture(t *testing.T, instance string, dir string, now func() time.Time) *Coordinator {
	t.Helper()
	if dir == "" {
		dir = filepath.Join(t.TempDir(), "state")
	}
	c, e := New(Config{Directory: dir, FilesystemPath: t.TempDir(), HostID: "host", FilesystemID: "disk", InstanceID: instance, MinFreeBytes: 1 << 20, CriticalFreeBytes: 1, MaxConcurrent: 1, Now: now})
	require.NoError(t, e)
	return c
}
func TestBudgetTwoInstancesCannotOverbookAndReleaseIsIdempotent(t *testing.T) {
	a := fixture(t, "prod", "", nil)
	b := fixture(t, "dev", a.cfg.Directory, nil)
	var wg sync.WaitGroup
	wins := make(chan *Lease, 2)
	errs := make(chan error, 2)
	for _, c := range []*Coordinator{a, b} {
		wg.Add(1)
		go func(c *Coordinator) {
			defer wg.Done()
			l, e := c.Reserve(context.Background(), "export", 1<<20, time.Minute)
			if e == nil {
				wins <- l
			} else {
				errs <- e
			}
		}(c)
	}
	wg.Wait()
	close(wins)
	close(errs)
	require.Len(t, wins, 1)
	require.Len(t, errs, 1)
	for l := range wins {
		require.NoError(t, l.Release(context.Background()))
		require.NoError(t, l.Release(context.Background()))
	}
	s, e := a.Status(context.Background())
	require.NoError(t, e)
	require.Empty(t, s.Active)
}
func TestBudgetCrashLeaseExpiresAndUnknownLedgerFailsClosed(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	c := fixture(t, "prod", "", clock)
	_, e := c.Reserve(context.Background(), "backup", 100, time.Minute)
	require.NoError(t, e)
	now = now.Add(2 * time.Minute)
	s, e := c.Status(context.Background())
	require.NoError(t, e)
	require.Zero(t, s.ReservedBytes)
	restarted := fixture(t, "prod", c.cfg.Directory, clock)
	l, e := restarted.Reserve(context.Background(), "backup", 100, time.Minute)
	require.NoError(t, e)
	require.NoError(t, l.Release(context.Background()))
	require.NoError(t, os.WriteFile(filepath.Join(c.cfg.Directory, "budget.json"), []byte(`{"version":99}`), 0600))
	_, e = c.Reserve(context.Background(), "backup", 100, time.Minute)
	require.ErrorIs(t, e, ErrUnavailable)
}
func TestBudgetPreservesFloorAndWrittenBytesAreNotReservedTwice(t *testing.T) {
	c := fixture(t, "prod", "", nil)
	s, e := c.Status(context.Background())
	require.NoError(t, e)
	_, e = c.Reserve(context.Background(), "backup", s.AvailableBytes, time.Minute)
	require.ErrorIs(t, e, ErrInsufficient)
	l, e := c.Reserve(context.Background(), "backup", 1000, time.Minute)
	require.NoError(t, e)
	require.NoError(t, l.Progress(context.Background(), 400, 1000, time.Minute))
	s, e = c.Status(context.Background())
	require.NoError(t, e)
	require.EqualValues(t, 600, s.ReservedBytes)
	require.ErrorIs(t, l.Progress(context.Background(), 400, s.AvailableBytes+1000, time.Minute), ErrInsufficient)
	require.NoError(t, l.Release(context.Background()))
}
func TestBudgetMetadataRejectsSymlinkAndForeignIdentity(t *testing.T) {
	c := fixture(t, "prod", "", nil)
	outside := filepath.Join(t.TempDir(), "outside")
	require.NoError(t, os.WriteFile(outside, []byte("private"), 0600))
	require.NoError(t, os.Symlink(outside, filepath.Join(c.cfg.Directory, "lifecycle.json")))
	_, e := c.Document(context.Background(), "lifecycle", func([]byte) ([]byte, error) { return []byte(`{}`), nil })
	require.ErrorIs(t, e, ErrUnavailable)
	_, e = c.Document(context.Background(), "../outside", nil)
	require.ErrorIs(t, e, ErrUnavailable)
	raw, e := os.ReadFile(outside)
	require.NoError(t, e)
	require.Equal(t, "private", string(raw))
}

func TestBudgetInitialPolicyIsAuthoritativeAcrossInstances(t *testing.T) {
	a := fixture(t, "prod", "", nil)
	b, err := New(Config{Directory: a.cfg.Directory, FilesystemPath: a.cfg.FilesystemPath, HostID: "host", FilesystemID: "disk", InstanceID: "dev", MinFreeBytes: 2 << 20, CriticalFreeBytes: 2, MaxConcurrent: 2})
	require.NoError(t, err)
	p, err := b.Policy(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 1<<20, p.MinFreeBytes)
	require.Equal(t, 1, p.MaxConcurrent)
	p.MinFreeBytes = 3 << 20
	p, err = b.SavePolicy(context.Background(), p.Revision, p)
	require.NoError(t, err)
	current, err := a.Policy(context.Background())
	require.NoError(t, err)
	require.Equal(t, p, current)
	_, err = a.SavePolicy(context.Background(), p.Revision-1, p)
	require.ErrorIs(t, err, ErrBusy)
}

func TestBudgetDirectoryReplacementAndLedgerSymlinksFailClosed(t *testing.T) {
	c := fixture(t, "prod", "", nil)
	ctx := context.Background()
	outside := filepath.Join(t.TempDir(), "outside")
	require.NoError(t, os.WriteFile(outside, []byte(`{"private":"must not read"}`), 0600))
	require.NoError(t, os.Remove(filepath.Join(c.cfg.Directory, "budget.json")))
	require.NoError(t, os.Symlink(outside, filepath.Join(c.cfg.Directory, "budget.json")))
	_, err := c.Status(ctx)
	require.ErrorIs(t, err, ErrUnavailable)
	require.NoError(t, os.Remove(filepath.Join(c.cfg.Directory, "budget.json")))
	old := c.cfg.Directory + "-old"
	require.NoError(t, os.Rename(c.cfg.Directory, old))
	t.Cleanup(func() { _ = os.RemoveAll(old) })
	require.NoError(t, os.Mkdir(c.cfg.Directory, 0700))
	_, err = c.Document(ctx, "alerts", func([]byte) ([]byte, error) { return []byte(`{}`), nil })
	require.ErrorIs(t, err, ErrUnavailable)
	require.NoFileExists(t, filepath.Join(c.cfg.Directory, "alerts.json"))
}
