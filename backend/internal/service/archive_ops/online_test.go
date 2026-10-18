package archive_ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPinnedMediaSurvivesAtomicReplaceAndDelete(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "a.jpg")
	require.NoError(t, os.WriteFile(file, []byte("A"), 0600))
	pinned, e := openReferencedFile(root, "a.jpg")
	require.NoError(t, e)
	defer pinned.Close()
	replacement := filepath.Join(root, "replacement")
	require.NoError(t, os.WriteFile(replacement, []byte("B"), 0600))
	require.NoError(t, os.Rename(replacement, file))
	require.NoError(t, os.Remove(file))
	raw, e := io.ReadAll(pinned)
	require.NoError(t, e)
	require.Equal(t, "A", string(raw))
}
func TestOnlineReferenceRejectsSymlinkAndDetectsChangedContent(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("private"), 0600))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "link")))
	_, e := openReferencedFile(root, "link/secret")
	require.Error(t, e)
	_, e = openReferencedFile(root, "../secret")
	require.Error(t, e)
	s := NewService(context.Background(), Config{Directory: t.TempDir()}, Options{})
	defer s.Close()
	sum := sha256.Sum256([]byte("A"))
	checksum := hex.EncodeToString(sum[:])
	_, _, e = s.captureReferencedFile(context.Background(), root, "removed.jpg", 1, checksum)
	require.ErrorIs(t, e, ErrSourceChanged)
	require.NoError(t, os.WriteFile(filepath.Join(root, "photo.jpg"), []byte("B"), 0600))
	_, _, e = s.captureReferencedFile(context.Background(), root, "photo.jpg", 1, checksum)
	require.ErrorIs(t, e, ErrSourceChanged)
}
