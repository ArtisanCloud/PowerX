package backup_ops

import (
	"context"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestQuarantineProbeRejectsUnsafePathsAndUnstableEvidence(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "fifo", "parent-link", "outside", "parent-missing", "arrived", "changed-restored-mtime", "removed"} {
		t.Run(kind, func(t *testing.T) {
			root, row, path, data := verifiedReviewFixture(t, 1<<20)
			switch kind {
			case "symlink":
				require.NoError(t, os.Remove(path))
				require.NoError(t, os.Symlink(path+".json", path))
			case "hardlink":
				require.NoError(t, os.Link(path, path+".extra"))
			case "fifo":
				require.NoError(t, os.Remove(path))
				require.NoError(t, unix.Mkfifo(path, 0600))
			case "parent-link":
				require.NoError(t, os.Symlink(root, filepath.Join(root, "alias")))
				row.StorageURI = "file://" + filepath.Join(root, "alias", "one.dump")
			case "outside":
				root = filepath.Join(root, "restricted")
			case "parent-missing":
				row.StorageURI = "file://" + filepath.Join(root, "absent", "one.dump")
			case "arrived":
				require.NoError(t, os.Remove(path))
			}
			committed := false
			changed := false
			out, digest, e := probeQuarantine(context.Background(), root, row, func(CleanupVerification) error {
				if changed {
					return nil
				}
				changed = true
				switch kind {
				case "arrived":
					require.NoError(t, os.WriteFile(path, data, 0600))
				case "changed-restored-mtime":
					info, err := os.Stat(path)
					require.NoError(t, err)
					require.NoError(t, os.WriteFile(path, make([]byte, len(data)), 0600))
					require.NoError(t, os.Chtimes(path, time.Now(), info.ModTime()))
				case "removed":
					require.NoError(t, os.Remove(path))
				}
				return nil
			}, func(string, string, func() bool) error { committed = true; return nil })
			require.NoError(t, e)
			require.False(t, committed)
			require.Empty(t, digest)
			require.False(t, out.ContentVerified)
			if kind == "parent-missing" {
				require.Equal(t, "unavailable", out.State)
			} else if kind == "arrived" || kind == "changed-restored-mtime" || kind == "removed" {
				require.Equal(t, "changed", out.State)
			} else {
				require.Equal(t, "unsafe", out.State)
			}
		})
	}
}
func TestQuarantineSnapshotChecksHeldDirectoryAndCommitTime(t *testing.T) {
	root, row, path, _ := verifiedReviewFixture(t, 32)
	require.NoError(t, os.Remove(path))
	out, digest, e := probeQuarantine(context.Background(), root, row, func(CleanupVerification) error { return nil }, func(kind, digest string, stable func() bool) error {
		require.Equal(t, "missing", kind)
		require.Len(t, digest, 64)
		require.True(t, stable())
		require.NoError(t, os.WriteFile(path, []byte("arrived after diagnosis"), 0600))
		require.False(t, stable())
		return nil
	})
	require.NoError(t, e)
	require.Equal(t, "missing", out.State)
	require.Len(t, digest, 64)
}
