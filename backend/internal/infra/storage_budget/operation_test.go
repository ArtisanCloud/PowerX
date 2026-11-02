package storagebudget

import (
	"bytes"
	"context"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestOperationNestedWritersShareLeaseExpandAndRelease(t *testing.T) {
	c := fixture(t, "prod", "", nil)
	SetDefault(c)
	t.Cleanup(func() { SetDefault(nil) })
	ctx, done, err := BeginPath(context.Background(), c.cfg.Directory, "snapshot", 4)
	require.NoError(t, err)
	defer done()
	nested, release, err := BeginPath(ctx, c.cfg.Directory, "archive", 8)
	require.NoError(t, err)
	release()
	status, err := c.Status(ctx)
	require.NoError(t, err)
	require.Len(t, status.Active, 1)
	var out bytes.Buffer
	_, err = Writer(nested, &out).Write([]byte("expanded safely"))
	require.NoError(t, err)
	require.Equal(t, "expanded safely", out.String())
	status, err = c.Status(ctx)
	require.NoError(t, err)
	require.Greater(t, status.ReservedBytes, int64(4))
	done()
	done()
	status, err = c.Status(context.Background())
	require.NoError(t, err)
	require.Empty(t, status.Active)
	_, err = Writer(ctx, &out).Write([]byte("after stop"))
	require.Error(t, err)
}
func TestOperationRefusesBeforeWritingWhenSharedBudgetCannotExpand(t *testing.T) {
	c := fixture(t, "prod", "", nil)
	SetDefault(c)
	t.Cleanup(func() { SetDefault(nil) })
	ctx, done, err := BeginPath(context.Background(), c.cfg.Directory, "upload", 1)
	require.NoError(t, err)
	defer done()
	p, err := c.Policy(ctx)
	require.NoError(t, err)
	p.MinFreeBytes = 1 << 50
	_, err = c.SavePolicy(ctx, p.Revision, p)
	require.NoError(t, err)
	var out bytes.Buffer
	n, err := Writer(ctx, &out).Write([]byte("must not write"))
	require.ErrorIs(t, err, ErrInsufficient)
	require.Zero(t, n)
	require.Empty(t, out.Bytes())
}
func TestBoundedEstimatorRejectsSymlinksRatherThanCountingUnknownAsZero(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "plain"), []byte("123"), 0600))
	n, err := TreeBytes(context.Background(), root)
	require.NoError(t, err)
	require.EqualValues(t, 3, n)
	require.NoError(t, os.Symlink(root, filepath.Join(root, "link")))
	_, err = TreeBytes(context.Background(), root)
	require.ErrorIs(t, err, ErrUnavailable)
}
