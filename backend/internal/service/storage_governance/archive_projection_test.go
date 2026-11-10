package storagegov

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	"github.com/stretchr/testify/require"
)

func TestArchiveControlProjectionWaitsOnlyForReadContention(t *testing.T) {
	dir := t.TempDir()
	c, err := budget.New(budget.Config{Directory: filepath.Join(dir, "ledger"), FilesystemPath: dir, HostID: "owned", FilesystemID: "owned", InstanceID: "owned", MinFreeBytes: 1024, CriticalFreeBytes: 1})
	require.NoError(t, err)
	for _, name := range []string{"archive-target-control", "archive-policy-control"} {
		t.Run(name, func(t *testing.T) {
			expected := []byte(`{"owned":true}`)
			_, err := c.Document(context.Background(), name, func([]byte) ([]byte, error) { return expected, nil })
			require.NoError(t, err)
			locked, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
			go func() {
				_, err := c.Document(context.Background(), name, func([]byte) ([]byte, error) { close(locked); <-release; return nil, nil })
				finished <- err
			}()
			<-locked
			released := false
			defer func() {
				if !released {
					close(release)
					<-finished
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			_, err = readArchiveControlProjection(ctx, c, name)
			cancel()
			require.ErrorIs(t, err, context.DeadlineExceeded)
			type result struct {
				raw []byte
				err error
			}
			read := make(chan result, 1)
			go func() {
				raw, err := readArchiveControlProjection(context.Background(), c, name)
				read <- result{raw, err}
			}()
			select {
			case <-read:
				t.Fatal("read passed an exclusively held projection lock")
			case <-time.After(20 * time.Millisecond):
			}
			close(release)
			released = true
			require.NoError(t, <-finished)
			out := <-read
			require.NoError(t, out.err)
			require.Equal(t, expected, out.raw)
			raw, err := c.Document(context.Background(), name, nil)
			require.NoError(t, err)
			require.Equal(t, expected, raw, "read contention must not change stored projection")
			calls := 0
			_, err = updateArchiveControlProjection(context.Background(), c, name, func([]byte) ([]byte, error) {
				calls++
				return nil, budget.ErrBusy
			})
			require.ErrorIs(t, err, budget.ErrBusy)
			require.Equal(t, 1, calls, "an entered publication must never be repeated")
		})
	}
}
