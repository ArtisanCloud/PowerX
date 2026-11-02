package archive_ops

import (
	"context"
	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	lifecycle "github.com/ArtisanCloud/PowerX/internal/infra/storage_lifecycle"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSealedStderrLogsRequireFreshRemoteProofBeforeSourceRemoval(t *testing.T) {
	credential, port := sshFixture(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	for _, item := range []struct {
		name string
		days int
	}{{"postgresql.log", 0}, {"postgresql.log.1.gz", 1}, {"postgresql.log.2.gz", 8}, {"postgresql.log.3.gz", 9}, {"business.txt", 30}} {
		file := filepath.Join(dir, item.name)
		require.NoError(t, os.WriteFile(file, []byte(item.name), 0600))
		at := time.Now().AddDate(0, 0, -item.days)
		require.NoError(t, os.Chtimes(file, at, at))
	}
	s := NewService(context.Background(), Config{Directory: t.TempDir(), Instance: "proof-test", Sources: []Source{{ID: "stderr", Kind: "postgresql_logs", Directory: dir, RotationFormat: "numeric", ActiveFiles: []string{"postgresql.log"}}}, Credentials: map[string]Credential{"loopback": credential}}, Options{})
	t.Cleanup(s.Close)
	target, err := s.SaveTarget(archiveRoot(), Target{Name: "loopback", Host: "127.0.0.1", Port: port, User: "archive", Directory: filepath.Join(t.TempDir(), "remote"), CredentialRef: "loopback"})
	require.NoError(t, err)
	require.NoError(t, s.TestTarget(archiveRoot(), target.ID))
	p, err := s.SavePolicy(archiveRoot(), Policy{Enabled: true, Name: "sealed stderr", SourceID: "stderr", IntervalMinutes: 60, RetentionDays: 7, RetentionCount: 3, RemoteRetentionDays: 90, RemoteRetentionCount: 90, TargetID: target.ID})
	require.NoError(t, err)
	j := captureTest(t, s, p)
	require.Equal(t, "success", j.Status, j.Error)
	require.Equal(t, "success", j.RemoteState, j.RemoteError)
	require.Len(t, j.Files, 2)
	for _, entry := range j.Files {
		require.NotContains(t, entry.Path, ".1.gz")
		require.NotContains(t, entry.Path, "business")
	}
	source := lifecycle.Source{ID: "stderr", Name: "stderr", Kind: "postgresql_logs", Scope: "prod", InstanceID: "proof-test", Directory: dir, Format: "numeric", ActiveFiles: []string{"postgresql.log"}, OwnerUID: uint32(os.Geteuid()), RequireArchive: true}
	coordinator, err := budget.New(budget.Config{Directory: filepath.Join(t.TempDir(), "state"), FilesystemPath: dir, HostID: "host", FilesystemID: "disk", InstanceID: "proof-test", MinFreeBytes: 1 << 20, CriticalFreeBytes: 1})
	require.NoError(t, err)
	engine, err := lifecycle.New(lifecycle.Options{Coordinator: coordinator, InstanceID: "proof-test", AllowEnforce: true, Sources: []lifecycle.Source{source}, VerifyArchive: func(ctx context.Context, src lifecycle.Source, c lifecycle.Candidate) (bool, error) {
		return s.VerifyLifecycleFile(ctx, src.Directory, c.Name, c.Bytes, time.Unix(0, c.ModifiedNS))
	}})
	require.NoError(t, err)
	_, err = engine.Save(context.Background(), 1, "stderr", 0, lifecycle.Policy{Enabled: true, RetentionDays: 7, MaxBytes: 1 << 20})
	require.NoError(t, err)
	entry := j.Files[0]
	remoteObject := filepath.Join(target.Directory, s.cfg.Instance, p.ID, "objects", entry.SHA256[:2], entry.SHA256)
	require.NoError(t, os.WriteFile(remoteObject, []byte("corrupted after upload"), 0600))
	blocked, err := engine.RunWithKey(context.Background(), "stderr", "corrupted-remote")
	require.NoError(t, err)
	require.Equal(t, "blocked", blocked.State)
	require.Zero(t, blocked.DeletedFiles)
	require.FileExists(t, filepath.Join(dir, "postgresql.log.2.gz"))
	raw, err := os.ReadFile(s.objectPath(entry.SHA256))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(remoteObject, raw, 0600))
	completed, err := engine.RunWithKey(context.Background(), "stderr", "verified-remote")
	require.NoError(t, err)
	require.Equal(t, 2, completed.DeletedFiles)
	require.Equal(t, "completed", completed.State)
	require.FileExists(t, filepath.Join(dir, "postgresql.log"))
	require.FileExists(t, filepath.Join(dir, "postgresql.log.1.gz"))
	require.FileExists(t, filepath.Join(dir, "business.txt"))
}
