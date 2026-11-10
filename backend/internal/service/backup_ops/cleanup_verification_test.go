package backup_ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func verifiedReviewFixture(t *testing.T, size int) (string, repo.CleanupReviewRow, string, []byte) {
	t.Helper()
	root, e := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, e)
	path := filepath.Join(root, "one.dump")
	data := []byte(strings.Repeat("v", size))
	sum := sha256.Sum256(data)
	row := repo.CleanupReviewRow{ID: 1, JobID: 2, PolicyID: 3, JobFound: true, CleanupState: "unconfirmed", CleanupToken: uuid.NewString(), StorageURI: "file://" + path, SizeBytes: int64(len(data)), Checksum: hex.EncodeToString(sum[:]), UpdatedAt: time.Now().UTC()}
	require.NoError(t, os.WriteFile(path, data, 0600))
	manifest := artifactManifest{Contract: "powerx.database-backup/v1", Database: "owned", JobID: row.JobID, PolicyID: row.PolicyID, File: filepath.Base(path), CreatedAt: time.Now().UTC(), Size: row.SizeBytes, SHA256: row.Checksum, Format: "postgresql-custom"}
	raw, e := json.Marshal(manifest)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(path+".json", raw, 0600))
	require.NoError(t, os.WriteFile(path+".sha256", []byte(row.Checksum+"  one.dump\n"), 0600))
	return root, row, path, data
}
func TestCleanupVerificationFullReadAndSidecars(t *testing.T) {
	root, row, path, data := verifiedReviewFixture(t, 9<<20)
	var previous int64
	calls := 0
	out, e := verifyCleanupContent(context.Background(), root, row, func(v CleanupVerification) error {
		calls++
		require.Equal(t, "running", v.State)
		require.False(t, v.ContentVerified)
		require.GreaterOrEqual(t, v.BytesRead, previous)
		previous = v.BytesRead
		return nil
	})
	require.NoError(t, e)
	require.Equal(t, "verified", out.State)
	require.True(t, out.ContentVerified)
	require.True(t, out.DumpMatched)
	require.True(t, out.ReadOnly)
	require.True(t, out.CleanupUnchanged)
	require.Equal(t, row.SizeBytes, out.BytesRead)
	require.GreaterOrEqual(t, calls, 3)
	got, e := os.ReadFile(path)
	require.NoError(t, e)
	require.Equal(t, data, got)
	require.Equal(t, "unconfirmed", row.CleanupState)
}
func TestCleanupVerificationFailureStates(t *testing.T) {
	for _, mode := range []string{"checksum", "size", "missing", "manifest", "manifest-policy", "checksum-sidecar", "sidecar-large", "sidecar-missing", "hardlink", "symlink", "parent-symlink", "fifo", "metadata"} {
		t.Run(mode, func(t *testing.T) {
			root, row, path, data := verifiedReviewFixture(t, 64)
			expected := "sidecar_mismatch"
			switch mode {
			case "checksum":
				data[0] = 'x'
				require.NoError(t, os.WriteFile(path, data, 0600))
				expected = "checksum_mismatch"
			case "size":
				require.NoError(t, os.WriteFile(path, []byte("short"), 0600))
				expected = "size_changed"
			case "missing":
				require.NoError(t, os.Remove(path))
				expected = "missing"
			case "manifest":
				require.NoError(t, os.WriteFile(path+".json", []byte(`{"contract":"other"}`), 0600))
			case "manifest-policy":
				raw, e := os.ReadFile(path + ".json")
				require.NoError(t, e)
				require.NoError(t, os.WriteFile(path+".json", []byte(strings.Replace(string(raw), `"policy_id":3`, `"policy_id":4`, 1)), 0600))
			case "checksum-sidecar":
				require.NoError(t, os.WriteFile(path+".sha256", []byte("wrong"), 0600))
			case "sidecar-large":
				require.NoError(t, os.WriteFile(path+".json", make([]byte, 65537), 0600))
			case "sidecar-missing":
				require.NoError(t, os.Remove(path+".sha256"))
				expected = "sidecars_missing"
			case "hardlink":
				require.NoError(t, os.Link(path, path+".extra"))
				expected = "unsafe"
			case "symlink":
				require.NoError(t, os.Remove(path+".json"))
				require.NoError(t, os.Symlink(path, path+".json"))
				expected = "unsafe"
			case "parent-symlink":
				parent := filepath.Join(root, "linked")
				require.NoError(t, os.Symlink(root, parent))
				row.StorageURI = "file://" + filepath.Join(parent, "one.dump")
				expected = "unsafe"
			case "fifo":
				require.NoError(t, os.Remove(path))
				require.NoError(t, unix.Mkfifo(path, 0600))
				expected = "unsafe"
			case "metadata":
				row.Checksum = "invalid"
				expected = "metadata_invalid"
			}
			out, e := verifyCleanupContent(context.Background(), root, row, func(CleanupVerification) error { return nil })
			require.NoError(t, e)
			require.Equal(t, expected, out.State)
			require.False(t, out.ContentVerified)
		})
	}
}
func TestCleanupVerificationCancellationAndReplacement(t *testing.T) {
	for _, mode := range []string{"cancel", "writer", "replace", "unlink", "grow"} {
		t.Run(mode, func(t *testing.T) {
			root, row, path, data := verifiedReviewFixture(t, 5<<20)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			changed := false
			out, e := verifyCleanupContent(ctx, root, row, func(v CleanupVerification) error {
				if changed {
					return nil
				}
				changed = true
				switch mode {
				case "cancel":
					cancel()
				case "writer":
					return errors.New("client disconnected")
				case "replace":
					require.NoError(t, os.Rename(path, path+".old"))
					require.NoError(t, os.WriteFile(path, data, 0600))
				case "unlink":
					require.NoError(t, os.Remove(path))
				case "grow":
					f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
					require.NoError(t, err)
					_, err = f.Write([]byte("growth"))
					require.NoError(t, err)
					f.Close()
				}
				return nil
			})
			require.False(t, out.ContentVerified)
			if mode == "cancel" {
				require.ErrorIs(t, e, context.Canceled)
			} else if mode == "writer" {
				require.ErrorContains(t, e, "client disconnected")
			} else {
				require.NoError(t, e)
				require.Equal(t, "changed", out.State)
			}
		})
	}
}
