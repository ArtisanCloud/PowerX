package archive_ops

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestArchiveNativePostgresUsesLiveManifestAndPreservesActiveLatestAndUnknown(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	dir := filepath.Join(root, "log")
	require.NoError(t, os.Mkdir(dir, 0700))
	names := []string{"postgresql-2026-01-04_010000.log", "postgresql-2026-01-03_010000.log", "postgresql-2026-01-02_010000.log", "business.txt"}
	for i, name := range names {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte("owned archive fixture"), 0600))
		at := time.Now().Add(-time.Duration(10+i) * 24 * time.Hour)
		require.NoError(t, os.Chtimes(path, at, at))
	}
	manifest := filepath.Join(root, "current_logfiles")
	require.NoError(t, os.WriteFile(manifest, []byte("stderr log/"+names[0]+"\n"), 0600))
	source := Source{ID: "pg", Kind: "postgresql_logs", Directory: dir, RotationFormat: "postgresql", ActiveFiles: []string{"postgresql-"}, CurrentLogfiles: manifest}
	cfg := Config{Directory: filepath.Join(root, "archives"), Instance: "native", Sources: []Source{source}}
	require.NoError(t, validateConfig(&cfg))
	svc := NewService(context.Background(), cfg, Options{})
	t.Cleanup(svc.Close)
	entries := map[string]Entry{}
	require.NoError(t, svc.captureSealedLogs(context.Background(), &jobRecord{}, source, entries))
	require.Len(t, entries, 1)
	require.NotEmpty(t, entries["postgresql_logs/"+names[2]].SourceSnapshot)
	for _, name := range names {
		require.FileExists(t, filepath.Join(dir, name))
	}
	// Missing collector evidence cannot fall back to archiving every file.
	require.NoError(t, os.Remove(manifest))
	err = svc.captureSealedLogs(context.Background(), &jobRecord{}, source, map[string]Entry{})
	require.Error(t, err)
	source.CurrentLogfiles = ""
	cfg.Sources = []Source{source}
	require.Error(t, validateConfig(&cfg))
}
