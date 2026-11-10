package archive_ops

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func interruptedFixture(t *testing.T, s *Service, p Policy, status, remote string) (jobRecord, string) {
	t.Helper()
	key := uuid.NewString()
	digest := sha256.Sum256([]byte(key))
	j := jobRecord{Job: Job{ID: uuid.NewString(), PolicyID: p.ID, PolicyRevision: p.Revision, SourceID: p.SourceID, Kind: "media", Status: status, Phase: "capturing", RemoteState: remote, CreatedAt: time.Now().Add(-48 * time.Hour)}, RequestKey: hex.EncodeToString(digest[:])}
	finish, e := s.beginExecution(&j)
	require.NoError(t, e)
	finish()
	require.NoError(t, s.saveJob(&j))
	return j, key
}

func TestExecutionOwnershipAndReadOnlyObservation(t *testing.T) {
	s, _, p := fixture(t)
	j, key := interruptedFixture(t, s, p, "running", "not_requested")
	finish, e := s.beginExecution(&j)
	require.NoError(t, e)
	defer finish()
	require.NoError(t, s.saveJob(&j))
	release, e := s.lock(archiveRoot(), p.ID)
	require.NoError(t, e)
	peer := NewService(context.Background(), s.cfg, Options{})
	t.Cleanup(peer.Close)
	require.NoError(t, peer.initErr)
	before, e := os.ReadFile(s.path("jobs", j.ID))
	require.NoError(t, e)
	got, e := s.Job(archiveRoot(), j.ID)
	require.NoError(t, e)
	require.Equal(t, "active", got.ExecutionState)
	got, e = peer.Job(archiveRoot(), j.ID)
	require.NoError(t, e)
	require.Equal(t, "unconfirmed", got.ExecutionState)
	receipt, e := peer.RunRequest(archiveRoot(), p.ID, key)
	require.NoError(t, e)
	require.Equal(t, j.ID, receipt.JobID)
	require.Equal(t, "unconfirmed", receipt.ExecutionState)
	after, e := os.ReadFile(s.path("jobs", j.ID))
	require.NoError(t, e)
	require.Equal(t, before, after, "GET and a busy peer must not rewrite evidence")
	// 即使同一个 job ID 仍在 map 中，回放旧执行代次也不能宣称当前活跃。
	j.ExecutionID = uuid.NewString()
	require.NoError(t, s.saveJob(&j))
	got, e = s.Job(archiveRoot(), j.ID)
	require.NoError(t, e)
	require.Equal(t, "unconfirmed", got.ExecutionState)
	release()
	require.NoError(t, peer.reconcileInterrupted(archiveRoot()))
	got, e = peer.Job(archiveRoot(), j.ID)
	require.NoError(t, e)
	require.Equal(t, "interrupted", got.Status)
	require.True(t, got.InterruptionPending)
	require.NotNil(t, got.InterruptedAt)
}

func TestInterruptedCaptureBlocksDueTimerNewTaskAndObjectGC(t *testing.T) {
	s, _, p := fixture(t)
	p.Enabled = true
	require.NoError(t, writeJSON(s.path("policies", p.ID), p))
	j, key := interruptedFixture(t, s, p, "running", "not_requested")
	body := []byte("partial capture has not published its Files list")
	sum := sha256.Sum256(body)
	object := s.objectPath(hex.EncodeToString(sum[:]))
	require.NoError(t, os.MkdirAll(filepath.Dir(object), 0700))
	require.NoError(t, os.WriteFile(object, body, 0600))
	restarted := NewService(context.Background(), s.cfg, Options{})
	t.Cleanup(restarted.Close)
	require.NoError(t, restarted.initErr)
	got, e := restarted.Job(archiveRoot(), j.ID)
	require.NoError(t, e)
	require.Equal(t, "interrupted", got.Status)
	// CreatedAt 已超过 interval，若只靠“未到周期”跳过，此断言不会成立。
	require.NoError(t, restarted.Tick(archiveRoot()))
	ids, e := s.records("jobs")
	require.NoError(t, e)
	require.Equal(t, []string{j.ID}, ids)
	_, e = restarted.Start(archiveRoot(), p.ID, uuid.NewString(), "root", "")
	require.Equal(t, 409, dto.StatusCode(e))
	replay, e := restarted.Start(archiveRoot(), p.ID, key, "root", "")
	require.NoError(t, e)
	require.Equal(t, j.ID, replay.ID)
	require.Equal(t, "interrupted", replay.ExecutionState)
	_, e = restarted.cleanupApproved(archiveRoot(), p.ID)
	require.NoError(t, e)
	after, e := os.ReadFile(object)
	require.NoError(t, e)
	require.Equal(t, body, after)
	_, e = restarted.Retry(archiveRoot(), j.ID)
	require.Equal(t, 409, dto.StatusCode(e))
	receipt, e := restarted.RunRequest(archiveRoot(), p.ID, key)
	require.NoError(t, e)
	require.True(t, receipt.InterruptionPending)
}

func TestReconcilePendingTransferAndMigrationLock(t *testing.T) {
	s, _, p := fixture(t)
	j, _ := interruptedFixture(t, s, p, "success", "pending")
	j.Phase = "uploading"
	j.MigratedFrom = uuid.NewString()
	require.NoError(t, s.saveJob(&j))
	release, e := s.lock(archiveRoot(), "migration-"+j.ID)
	require.NoError(t, e)
	require.NoError(t, s.reconcileInterrupted(archiveRoot()))
	original, e := s.readJob(j.ID)
	require.NoError(t, e)
	require.Equal(t, "pending", original.RemoteState)
	release()
	require.NoError(t, s.reconcileInterrupted(archiveRoot()))
	got, e := s.Job(archiveRoot(), j.ID)
	require.NoError(t, e)
	require.Equal(t, "success", got.Status)
	require.Equal(t, "unconfirmed", got.RemoteState)
	require.Equal(t, "uploading", got.InterruptedPhase)
	require.True(t, got.InterruptionPending)
	complete, _ := interruptedFixture(t, s, p, "success", "success")
	require.NoError(t, s.reconcileInterrupted(archiveRoot()))
	got, e = s.Job(archiveRoot(), complete.ID)
	require.NoError(t, e)
	require.False(t, got.InterruptionPending)
	require.Equal(t, "recorded", got.ExecutionState)
}

func TestExecutionAttentionSurvivesRecentJobTruncation(t *testing.T) {
	s, _, p := fixture(t)
	old, _ := interruptedFixture(t, s, p, "running", "not_requested")
	require.NoError(t, s.reconcileInterrupted(archiveRoot()))
	for i := 0; i < 101; i++ {
		j, _ := interruptedFixture(t, s, p, "success", "not_requested")
		j.CreatedAt = time.Now()
		require.NoError(t, s.saveJob(&j))
	}
	view, e := s.Overview(archiveRoot())
	require.NoError(t, e)
	require.Len(t, view.Jobs, 100)
	require.Len(t, view.ExecutionAttention, 1)
	require.Equal(t, old.ID, view.ExecutionAttention[0].ID)
	require.Equal(t, p.ID, view.ExecutionAttention[0].PolicyID)
}

func TestRestartHistoricalRemoteSuccessStillRequiresFreshProof(t *testing.T) {
	s, peer, _, p := s3Fixture(t)
	p.Enabled = true
	require.NoError(t, s.TestTarget(archiveRoot(), p.TargetID))
	var e error
	p, e = s.SavePolicy(archiveRoot(), p)
	require.NoError(t, e)
	j := captureTest(t, s, p)
	require.Equal(t, "success", j.RemoteState)
	s.Close()
	restarted := NewService(context.Background(), s.cfg, Options{})
	t.Cleanup(restarted.Close)
	src := restarted.cfg.Sources[0]
	file, e := os.Stat(filepath.Join(src.Directory, "app.log.2.gz"))
	require.NoError(t, e)
	proof, e := restarted.VerifyLifecycleFile(archiveRoot(), src.Directory, file.Name(), file.Size(), file.ModTime())
	require.NoError(t, e)
	require.True(t, proof)
	peer.mu.Lock()
	peer.corrupt = true
	peer.mu.Unlock()
	proof, _ = restarted.VerifyLifecycleFile(archiveRoot(), src.Directory, file.Name(), file.Size(), file.ModTime())
	require.False(t, proof, "a recorded successful upload cannot replace current remote verification")
	got, e := restarted.Job(archiveRoot(), j.ID)
	require.NoError(t, e)
	require.Equal(t, "success", got.RemoteState, "historical fact is not rewritten as current proof")
}

func TestArchiveManifestDoesNotExportCurrentExecutionGate(t *testing.T) {
	at := time.Now().UTC()
	j := Job{ID: uuid.NewString(), ExecutionState: "active", InterruptionPending: true, InterruptedAt: &at, InterruptedPhase: "uploading"}
	out := archiveManifest(j)
	require.Empty(t, out.ExecutionState)
	require.False(t, out.InterruptionPending)
	require.Equal(t, j.InterruptedAt, out.InterruptedAt)
	require.True(t, j.InterruptionPending)
}

func TestDownloadManifestDoesNotExportLocalExecutionEvidence(t *testing.T) {
	s, _, p := fixture(t)
	j := captureTest(t, s, p)
	var out bytes.Buffer
	require.NoError(t, s.Download(archiveRoot(), j.ID, &out))
	gz, e := gzip.NewReader(&out)
	require.NoError(t, e)
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, e := tr.Next()
		require.NoError(t, e)
		if h.Name != "archive-manifest.json" {
			continue
		}
		b, e := io.ReadAll(tr)
		require.NoError(t, e)
		require.Contains(t, string(b), j.ID)
		require.NotContains(t, string(b), "execution_id")
		require.NotContains(t, string(b), "request_key")
		require.NotContains(t, string(b), "interruption_pending")
		break
	}
}
