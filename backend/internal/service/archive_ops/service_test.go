package archive_ops

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func archiveRoot() context.Context { return reqctx.WithIsRoot(context.Background(), true) }
func fixture(t *testing.T) (*Service, string, Policy) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.jpg"), []byte("same"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.jpg"), []byte("same"), 0600))
	s := NewService(context.Background(), Config{Directory: t.TempDir(), Instance: "test", Sources: []Source{{ID: "media", Kind: "media", Directory: dir}}}, Options{})
	t.Cleanup(s.Close)
	p, e := s.SavePolicy(archiveRoot(), Policy{Name: "media", SourceID: "media", IntervalMinutes: 60, RetentionDays: 7, RetentionCount: 3, RemoteRetentionDays: 90, RemoteRetentionCount: 90})
	require.NoError(t, e)
	return s, dir, p
}
func captureTest(t *testing.T, s *Service, p Policy) Job {
	t.Helper()
	j, e := s.Start(archiveRoot(), p.ID, uuid.NewString(), "test.root", uuid.NewString())
	require.NoError(t, e)
	require.Eventually(t, func() bool { j, e := s.Job(archiveRoot(), j.ID); return e == nil && j.Status != "running" }, 10*time.Second, 20*time.Millisecond)
	require.Eventually(t, func() bool {
		u, e := s.lock(archiveRoot(), p.ID)
		if e != nil {
			return false
		}
		u()
		return true
	}, 40*time.Second, 20*time.Millisecond)
	j, e = s.Job(archiveRoot(), j.ID)
	require.NoError(t, e)
	return j
}
func TestMediaIncrementalRestoreAndRetention(t *testing.T) {
	s, dir, p := fixture(t)
	first := captureTest(t, s, p)
	require.Equal(t, "success", first.Status, first.Error)
	require.Equal(t, 2, first.FileCount)
	require.EqualValues(t, 4, first.NewBytes)
	require.EqualValues(t, 8, first.SizeBytes)
	second := captureTest(t, s, p)
	require.EqualValues(t, 0, second.NewBytes)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.jpg"), []byte("changed"), 0600))
	restore, e := s.StartRestore(archiveRoot(), first.ID, false, "root", "")
	require.NoError(t, e)
	require.Eventually(t, func() bool {
		r, e := s.RestoreDirectory(archiveRoot(), restore.ID)
		return e == nil && r.Status != "running"
	}, 5*time.Second, 20*time.Millisecond)
	r, e := s.RestoreDirectory(archiveRoot(), restore.ID)
	require.NoError(t, e)
	require.Equal(t, "success", r.Status, r.Error)
	data, e := os.ReadFile(filepath.Join(r.Directory, "media/a.jpg"))
	require.NoError(t, e)
	require.Equal(t, "same", string(data))
	data, e = os.ReadFile(filepath.Join(dir, "a.jpg"))
	require.NoError(t, e)
	require.Equal(t, "changed", string(data))
	require.NoError(t, s.RemoveRestore(archiveRoot(), r.ID, r.ID))
	_, e = os.Stat(filepath.Join(dir, "a.jpg"))
	require.NoError(t, e)
	var download bytes.Buffer
	require.NoError(t, s.Download(archiveRoot(), first.ID, &download))
	gz, e := gzip.NewReader(&download)
	require.NoError(t, e)
	tarReader := tar.NewReader(gz)
	names := []string{}
	for {
		h, e := tarReader.Next()
		if e == io.EOF {
			break
		}
		require.NoError(t, e)
		names = append(names, h.Name)
	}
	require.Contains(t, names, "archive-manifest.json")
	require.Contains(t, names, "media/a.jpg")
	_, e = s.Protect(archiveRoot(), first.ID, true)
	require.NoError(t, e)
	p.RetentionCount = 1
	p, e = s.SavePolicy(archiveRoot(), p)
	require.NoError(t, e)
	third := captureTest(t, s, p)
	require.Equal(t, "success", third.Status, third.Error)
	require.Eventually(t, func() bool { j, e := s.Job(archiveRoot(), second.ID); return e == nil && j.Status == "pruned" }, 5*time.Second, 20*time.Millisecond)
	protected, e := s.Job(archiveRoot(), first.ID)
	require.NoError(t, e)
	require.Equal(t, "success", protected.Status)
}
func TestRootAndSourceBoundary(t *testing.T) {
	s, dir, p := fixture(t)
	_, e := s.Overview(context.Background())
	require.Equal(t, 403, dto.StatusCode(e))
	require.NoError(t, os.Symlink(filepath.Join(dir, "a.jpg"), filepath.Join(dir, "escape")))
	j := captureTest(t, s, p)
	require.Equal(t, "failed", j.Status)
	require.Contains(t, j.Error, "软链接")
	_, e = s.SaveTarget(archiveRoot(), Target{Name: "bad", Host: "localhost; echo injected", Port: 22, User: "root", Directory: "/data/archive", CredentialRef: "test"})
	require.Equal(t, 400, dto.StatusCode(e))
	_, e = s.SavePolicy(archiveRoot(), Policy{Name: "x", SourceID: "../../etc", IntervalMinutes: 1, RetentionDays: 1, RetentionCount: 1, RemoteRetentionDays: 1, RemoteRetentionCount: 1})
	require.Equal(t, 400, dto.StatusCode(e))
}
func TestRestoreRejectsChangedDirectoryIdentity(t *testing.T) {
	s, _, p := fixture(t)
	j := captureTest(t, s, p)
	r, e := s.StartRestore(archiveRoot(), j.ID, false, "root", "")
	require.NoError(t, e)
	require.Eventually(t, func() bool { r, e := s.RestoreDirectory(archiveRoot(), r.ID); return e == nil && r.Status == "success" }, 5*time.Second, 20*time.Millisecond)
	r, e = s.RestoreDirectory(archiveRoot(), r.ID)
	require.NoError(t, e)
	require.NoError(t, os.Rename(r.Directory, r.Directory+"-original"))
	require.NoError(t, os.Mkdir(r.Directory, 0700))
	e = s.RemoveRestore(archiveRoot(), r.ID, r.ID)
	require.Equal(t, 409, dto.StatusCode(e))
	_, e = os.Stat(r.Directory + "-original/media/a.jpg")
	require.NoError(t, e)
}
func TestIdempotencyAndSourceFileIntegrity(t *testing.T) {
	s, _, p := fixture(t)
	key := uuid.NewString()
	j, e := s.Start(archiveRoot(), p.ID, key, "root", "")
	require.NoError(t, e)
	require.Eventually(t, func() bool { j, e := s.Job(archiveRoot(), j.ID); return e == nil && j.Status == "success" }, 5*time.Second, 20*time.Millisecond)
	same, e := s.Start(archiveRoot(), p.ID, key, "root", "")
	require.NoError(t, e)
	require.Equal(t, j.ID, same.ID)
	j, e = s.Job(archiveRoot(), j.ID)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(s.objectPath(j.Files[0].SHA256), []byte("bad"), 0600))
	var output bytes.Buffer
	require.Error(t, s.Download(archiveRoot(), j.ID, &output))
	require.Zero(t, output.Len())
}
func TestIndependentTimerReportsCaptureFailure(t *testing.T) {
	s, dir, p := fixture(t)
	p.Enabled = true
	_, e := s.SavePolicy(archiveRoot(), p)
	require.NoError(t, e)
	require.NoError(t, os.Symlink(filepath.Join(dir, "a.jpg"), filepath.Join(dir, "unsafe-link")))
	e = s.Tick(context.Background())
	require.Error(t, e)
	require.Contains(t, e.Error(), "软链接")
	status, e := s.Overview(archiveRoot())
	require.NoError(t, e)
	require.Len(t, status.Jobs, 1)
	require.Equal(t, "failed", status.Jobs[0].Status)
}
