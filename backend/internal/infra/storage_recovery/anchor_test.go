package storagerecovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func privateAnchorDirectory(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "maintenance")
	require.NoError(t, os.Mkdir(dir, 0700))
	return dir
}
func TestAnchorDurableEpochAndReplay(t *testing.T) {
	dir := privateAnchorDirectory(t)
	ctx := context.Background()
	id := uuid.NewString()
	s, e := Initialize(dir, id)
	require.NoError(t, e)
	initial, e := s.Read(ctx)
	require.NoError(t, e)
	require.Empty(t, initial.Epoch)
	job, snapshot := uuid.NewString(), uuid.NewString()
	a, e := s.Begin(ctx, job, snapshot)
	require.NoError(t, e)
	require.NotEmpty(t, a.Epoch)
	reopened, e := Open(dir, id)
	require.NoError(t, e)
	b, e := reopened.Begin(ctx, job, snapshot)
	require.NoError(t, e)
	require.Equal(t, a, b)
	_, e = reopened.Begin(ctx, job, uuid.NewString())
	require.Error(t, e)
	b, e = reopened.Begin(ctx, uuid.NewString(), snapshot)
	require.NoError(t, e)
	require.NotEqual(t, a.Epoch, b.Epoch)
	require.Equal(t, a.AnchorID, b.AnchorID)
	_, e = Open(dir, uuid.NewString())
	require.Error(t, e)
}
func TestAnchorSharedStepBlocksEpochReplacement(t *testing.T) {
	s, e := Initialize(privateAnchorDirectory(t), uuid.NewString())
	require.NoError(t, e)
	ctx := context.Background()
	require.NoError(t, s.WithEpoch(ctx, func(Epoch) error {
		_, e := s.Begin(ctx, uuid.NewString(), uuid.NewString())
		require.ErrorIs(t, e, ErrBusy)
		return nil
	}))
	_, e = s.Begin(ctx, uuid.NewString(), uuid.NewString())
	require.NoError(t, e)
}
func TestAnchorNeverRecreatesMissingOrUnsafeProof(t *testing.T) {
	for _, kind := range []string{"missing", "mode", "symlink", "hardlink", "malformed"} {
		t.Run(kind, func(t *testing.T) {
			dir := privateAnchorDirectory(t)
			id := uuid.NewString()
			_, e := Initialize(dir, id)
			require.NoError(t, e)
			path := filepath.Join(dir, "storage-recovery-epoch.json")
			switch kind {
			case "missing":
				require.NoError(t, os.Remove(path))
			case "mode":
				require.NoError(t, os.Chmod(path, 0644))
			case "symlink":
				require.NoError(t, os.Remove(path))
				require.NoError(t, os.Symlink(filepath.Join(dir, "storage-recovery-binding.json"), path))
			case "hardlink":
				require.NoError(t, os.Link(path, filepath.Join(dir, "other.json")))
			case "malformed":
				require.NoError(t, os.WriteFile(path, []byte(`{"protocol":"other"}`), 0600))
			}
			_, e = Open(dir, id)
			require.Error(t, e)
			_, e = Initialize(dir, id)
			require.Error(t, e)
		})
	}
}
