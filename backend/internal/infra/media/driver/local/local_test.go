package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
)

func TestDeleteRemovesEmptyParentDirs(t *testing.T) {
	base := t.TempDir()
	d, err := New(Options{BasePath: base})
	require.NoError(t, err)

	objectPath := filepath.Join(base, "asset-1", "preview")
	require.NoError(t, os.MkdirAll(filepath.Dir(objectPath), 0o755))
	require.NoError(t, os.WriteFile(objectPath, []byte("preview"), 0o644))

	err = d.Delete(context.Background(), driver.DeleteObjectInput{ObjectKey: "asset-1/preview"})
	require.NoError(t, err)

	require.NoFileExists(t, objectPath)
	require.NoDirExists(t, filepath.Join(base, "asset-1"))
	require.DirExists(t, base)
}

func TestDeleteKeepsNonEmptyParentDir(t *testing.T) {
	base := t.TempDir()
	d, err := New(Options{BasePath: base})
	require.NoError(t, err)

	objectPath := filepath.Join(base, "asset-1", "preview")
	originPath := filepath.Join(base, "asset-1", "origin")
	require.NoError(t, os.MkdirAll(filepath.Dir(objectPath), 0o755))
	require.NoError(t, os.WriteFile(objectPath, []byte("preview"), 0o644))
	require.NoError(t, os.WriteFile(originPath, []byte("origin"), 0o644))

	err = d.Delete(context.Background(), driver.DeleteObjectInput{ObjectKey: "asset-1/preview"})
	require.NoError(t, err)

	require.NoFileExists(t, objectPath)
	require.FileExists(t, originPath)
	require.DirExists(t, filepath.Join(base, "asset-1"))
}
