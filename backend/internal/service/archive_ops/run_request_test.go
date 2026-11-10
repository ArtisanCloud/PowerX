package archive_ops

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRunRequestReadOnlySurvivesPolicyPauseRemovalAndRestart(t *testing.T) {
	s, _, p := fixture(t)
	key := uuid.NewString()
	job, e := s.StartAtRevision(archiveRoot(), p.ID, key, p.Revision, "owned-root", "owned-trace")
	require.NoError(t, e)
	require.Eventually(t, func() bool { j, e := s.Job(archiveRoot(), job.ID); return e == nil && j.Status != "running" }, 5*time.Second, 10*time.Millisecond)
	s.Close()
	// 以同一自有归档根重建服务；当前策略和业务 DB 均不作为回执读取前置。
	require.NoError(t, os.Remove(s.path("policies", p.ID)))
	reopened := NewService(context.Background(), s.cfg, s.opts)
	t.Cleanup(reopened.Close)
	before, e := os.ReadFile(s.path("jobs", job.ID))
	require.NoError(t, e)
	out, e := reopened.RunRequest(archiveRoot(), p.ID, key)
	require.NoError(t, e)
	require.Equal(t, key, out.RequestID)
	require.Equal(t, job.ID, out.JobID)
	require.Equal(t, p.Revision, out.PolicyRevision)
	require.Equal(t, p.SourceID, out.SourceID)
	after, e := os.ReadFile(s.path("jobs", job.ID))
	require.NoError(t, e)
	require.Equal(t, before, after)
	raw, e := json.Marshal(out)
	require.NoError(t, e)
	for _, forbidden := range []string{"request_key", "target", "credential", "files", "directory", "operator"} {
		require.NotContains(t, string(raw), `"`+forbidden+`"`)
	}
	ids, e := reopened.records("jobs")
	require.NoError(t, e)
	require.Len(t, ids, 1)
}

func TestRunRequestMissingCorruptConflictingAndDuplicateRemainUnconfirmed(t *testing.T) {
	s, _, p := fixture(t)
	ctx := archiveRoot()
	_, e := s.RunRequest(context.Background(), p.ID, uuid.NewString())
	require.Equal(t, "ARCHIVE_ROOT_REQUIRED", dto.CodeOf(e))
	_, e = s.RunRequest(ctx, p.ID, "bad")
	require.Equal(t, "ARCHIVE_REQUEST_ID_INVALID", dto.CodeOf(e))
	key := uuid.NewString()
	_, e = s.RunRequest(ctx, p.ID, key)
	require.Equal(t, "ARCHIVE_REQUEST_UNCONFIRMED", dto.CodeOf(e))
	job, e := s.StartAtRevision(ctx, p.ID, key, p.Revision, "owned", "")
	require.NoError(t, e)
	require.Eventually(t, func() bool { j, e := s.Job(ctx, job.ID); return e == nil && j.Status != "running" }, 5*time.Second, 10*time.Millisecond)
	_, e = s.RunRequest(ctx, uuid.NewString(), key)
	require.Equal(t, "ARCHIVE_REQUEST_CONTEXT_CONFLICT", dto.CodeOf(e))
	original, e := s.readJob(job.ID)
	require.NoError(t, e)
	duplicate := original
	duplicate.ID = uuid.NewString()
	require.NoError(t, s.saveJob(&duplicate))
	_, e = s.RunRequest(ctx, p.ID, key)
	require.Equal(t, "ARCHIVE_REQUEST_AMBIGUOUS", dto.CodeOf(e))
	require.NoError(t, os.Remove(s.path("jobs", duplicate.ID)))
	incomplete := original
	incomplete.PolicyRevision = 0
	require.NoError(t, s.saveJob(&incomplete))
	_, e = s.RunRequest(ctx, p.ID, key)
	require.Equal(t, "ARCHIVE_REQUEST_EVIDENCE_INCOMPLETE", dto.CodeOf(e))
	require.NoError(t, s.saveJob(&original))
	broken := uuid.NewString()
	require.NoError(t, os.WriteFile(s.path("jobs", broken), []byte("{broken"), 0600))
	_, e = s.RunRequest(ctx, p.ID, key)
	require.Equal(t, "ARCHIVE_REQUEST_LOOKUP_UNAVAILABLE", dto.CodeOf(e))
	require.NoError(t, os.Remove(s.path("jobs", broken)))
	require.NoError(t, os.Rename(filepath.Join(s.cfg.Directory, "jobs"), filepath.Join(s.cfg.Directory, "jobs-offline")))
	_, e = s.RunRequest(ctx, p.ID, key)
	require.Equal(t, "ARCHIVE_REQUEST_LOOKUP_UNAVAILABLE", dto.CodeOf(e))
	require.NoError(t, os.Rename(filepath.Join(s.cfg.Directory, "jobs-offline"), filepath.Join(s.cfg.Directory, "jobs")))
	out, e := s.RunRequest(ctx, p.ID, key)
	require.NoError(t, e)
	require.Equal(t, job.ID, out.JobID)
}

func TestRunRequestVersionConflictCreatesNoTaskAndConcurrentReplayKeepsOne(t *testing.T) {
	s, _, p := fixture(t)
	ctx := archiveRoot()
	key := uuid.NewString()
	_, e := s.StartAtRevision(ctx, p.ID, key, p.Revision+1, "owned", "")
	require.Equal(t, "ARCHIVE_POLICY_REVISION_CONFLICT", dto.CodeOf(e))
	ids, e := s.records("jobs")
	require.NoError(t, e)
	require.Empty(t, ids)
	job, e := s.StartAtRevision(ctx, p.ID, key, p.Revision, "owned", "")
	require.NoError(t, e)
	var group sync.WaitGroup
	results := make(chan Job, 8)
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			same, e := s.StartAtRevision(ctx, p.ID, key, p.Revision, "owned", "")
			results <- same
			errors <- e
		}()
	}
	group.Wait()
	close(results)
	close(errors)
	for e := range errors {
		require.NoError(t, e)
	}
	for same := range results {
		require.Equal(t, job.ID, same.ID)
	}
	ids, e = s.records("jobs")
	require.NoError(t, e)
	require.Len(t, ids, 1)
	changed, e := s.SavePolicy(ctx, p)
	require.NoError(t, e)
	_, e = s.StartAtRevision(ctx, p.ID, key, changed.Revision, "owned", "")
	require.Equal(t, "ARCHIVE_IDEMPOTENCY_CONFLICT", dto.CodeOf(e), "the same UUID cannot silently bind another policy revision")
}
