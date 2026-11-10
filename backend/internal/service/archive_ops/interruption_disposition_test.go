package archive_ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	recovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func dispositionFixture(t *testing.T) (*Service, Policy, jobRecord, string) {
	t.Helper()
	s, _, p := fixture(t)
	dir := filepath.Join(t.TempDir(), "maintenance")
	require.NoError(t, os.Mkdir(dir, 0700))
	anchor, e := recovery.Initialize(dir, uuid.NewString())
	require.NoError(t, e)
	require.NoError(t, s.BindRecoveryAnchor(anchor))
	j, key := interruptedFixture(t, s, p, "running", "not_requested")
	require.NoError(t, s.reconcileInterrupted(credentialRootContext()))
	j, e = s.readJob(j.ID)
	require.NoError(t, e)
	b := []byte("partial CAS without a complete Files manifest")
	sum := sha256.Sum256(b)
	object := s.objectPath(hex.EncodeToString(sum[:]))
	require.NoError(t, os.MkdirAll(filepath.Dir(object), 0700))
	require.NoError(t, os.WriteFile(object, b, 0600))
	return s, p, j, key
}
func retainInput(v InterruptionReview) RetainInterruptedInput {
	return RetainInterruptedInput{uuid.NewString(), v.ReviewVersion, dispositionAction, "人工核对中断采集，保留全部现存内容并结束本次任务", true}
}
func TestInterruptionRetainClosePreservesPartialCASAndOriginalIdentity(t *testing.T) {
	s, p, j, key := dispositionFixture(t)
	ctx := credentialRootContext()
	before := restartObjects(t, s.cfg.Directory)
	raw, e := os.ReadFile(s.path("jobs", j.ID))
	require.NoError(t, e)
	review, e := s.ReviewInterruptedCapture(ctx, j.ID, nil)
	require.NoError(t, e)
	require.Equal(t, 1, review.ObjectCount)
	after, e := os.ReadFile(s.path("jobs", j.ID))
	require.NoError(t, e)
	require.Equal(t, raw, after)
	in := retainInput(review)
	receipt, e := s.RetainInterruptedCapture(ctx, j.ID, in)
	require.NoError(t, e)
	require.Equal(t, "retained", receipt.State)
	require.True(t, receipt.CurrentRecordMatches)
	current, e := s.Job(ctx, j.ID)
	require.NoError(t, e)
	require.Equal(t, "retained", current.Status)
	require.True(t, current.Protected)
	require.False(t, current.InterruptionPending)
	require.Empty(t, current.Files)
	require.Equal(t, in.RequestID, current.CaptureRetention.RequestID)
	replay, e := s.RetainInterruptedCapture(ctx, j.ID, in)
	require.NoError(t, e)
	require.Equal(t, receipt.Result, replay.Result)
	_, e = s.Protect(ctx, j.ID, false)
	require.Equal(t, 409, dto.StatusCode(e))
	_, e = s.Retry(ctx, j.ID)
	require.Equal(t, 409, dto.StatusCode(e))
	_, e = s.cleanupApproved(ctx, p.ID)
	require.NoError(t, e)
	require.Equal(t, before, restartObjects(t, s.cfg.Directory))
	orphan := []byte("new unreferenced content outside the retained set")
	sum := sha256.Sum256(orphan)
	orphanPath := s.objectPath(hex.EncodeToString(sum[:]))
	require.NoError(t, os.MkdirAll(filepath.Dir(orphanPath), 0700))
	require.NoError(t, os.WriteFile(orphanPath, orphan, 0600))
	_, e = s.cleanupApproved(ctx, p.ID)
	require.NoError(t, e)
	_, e = os.Stat(orphanPath)
	require.True(t, os.IsNotExist(e))
	require.Equal(t, before, restartObjects(t, s.cfg.Directory))
	r, e := s.RunRequest(ctx, p.ID, key)
	require.NoError(t, e)
	require.Equal(t, j.ID, r.JobID)
	require.Equal(t, "retained", r.Status)
	all, e := s.records("jobs")
	require.NoError(t, e)
	require.Len(t, all, 1, "no automatic new capture")
	newer, e := s.Start(ctx, p.ID, uuid.NewString(), "root", "")
	require.NoError(t, e)
	require.NotEqual(t, j.ID, newer.ID, "a later explicit media capture is independent")
}
func TestInterruptionRetainRejectsStaleContentRecoveryAndBusyExecution(t *testing.T) {
	for _, scene := range []string{"content", "epoch", "policy", "busy", "symlink", "hardlink", "cancel"} {
		t.Run(scene, func(t *testing.T) {
			s, p, j, _ := dispositionFixture(t)
			ctx := credentialRootContext()
			v, e := s.ReviewInterruptedCapture(ctx, j.ID, nil)
			require.NoError(t, e)
			in := retainInput(v)
			switch scene {
			case "content":
				for path := range restartObjects(t, s.cfg.Directory) {
					require.NoError(t, os.WriteFile(s.cfg.Directory+path, []byte("corrupted"), 0600))
				}
			case "epoch":
				_, e = s.recoveryAnchor.Begin(ctx, uuid.NewString(), uuid.NewString())
				require.NoError(t, e)
			case "policy":
				p.Enabled = true
				_, e = s.SavePolicy(ctx, p)
				require.NoError(t, e)
			case "busy":
				r, e := s.lock(ctx, p.ID)
				require.NoError(t, e)
				defer r()
			case "symlink":
				require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(s.cfg.Directory, "objects", "ab")))
			case "hardlink":
				for path := range restartObjects(t, s.cfg.Directory) {
					require.NoError(t, os.Link(s.cfg.Directory+path, filepath.Join(t.TempDir(), "linked")))
				}
			case "cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			_, e = s.RetainInterruptedCapture(ctx, j.ID, in)
			require.Error(t, e)
			got, e := s.Job(credentialRootContext(), j.ID)
			require.NoError(t, e)
			require.True(t, got.InterruptionPending)
			require.Nil(t, got.CaptureRetention)
		})
	}
}
func TestInterruptionRequestConflictUnknownAndConcurrentReplay(t *testing.T) {
	s, _, j, _ := dispositionFixture(t)
	ctx := credentialRootContext()
	v, e := s.ReviewInterruptedCapture(ctx, j.ID, nil)
	require.NoError(t, e)
	in := retainInput(v)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := s.RetainInterruptedCapture(ctx, j.ID, in); errs <- e }()
	}
	wg.Wait()
	close(errs)
	success := 0
	for e := range errs {
		if e == nil {
			success++
		} else {
			require.Equal(t, 409, dto.StatusCode(e))
		}
	}
	require.Greater(t, success, 0)
	out, e := s.InterruptionRequest(ctx, in.RequestID)
	require.NoError(t, e)
	require.Equal(t, "retained", out.State)
	changed := in
	changed.Reason = "another input is not the original request"
	_, e = s.RetainInterruptedCapture(ctx, j.ID, changed)
	require.Equal(t, 409, dto.StatusCode(e))
	// 丢失最终回复只 GET 原 UUID；回放旧任务后，历史结论不能重做保护或结束当前中断。
	require.NoError(t, s.saveJob(&j))
	old, e := s.InterruptionRequest(ctx, in.RequestID)
	require.NoError(t, e)
	require.Equal(t, "retained", old.State)
	require.False(t, old.CurrentRecordMatches)
	_, e = s.RetainInterruptedCapture(ctx, j.ID, in)
	require.NoError(t, e)
	current, e := s.Job(ctx, j.ID)
	require.NoError(t, e)
	require.True(t, current.InterruptionPending)
}
func TestInterruptionRetainedMetadataCorruptionStopsGCAndRequestLookup(t *testing.T) {
	s, p, j, _ := dispositionFixture(t)
	ctx := credentialRootContext()
	v, e := s.ReviewInterruptedCapture(ctx, j.ID, nil)
	require.NoError(t, e)
	in := retainInput(v)
	_, e = s.RetainInterruptedCapture(ctx, j.ID, in)
	require.NoError(t, e)
	held, e := s.readJob(j.ID)
	require.NoError(t, e)
	held.RetainedObjects = nil
	require.NoError(t, s.saveJob(&held))
	_, e = s.cleanupApproved(ctx, p.ID)
	require.Error(t, e)
	_, e = s.InterruptionRequest(ctx, in.RequestID)
	require.Error(t, e)
}

func TestInterruptionEmptyCaptureAndRegisteredUnknownNeverReexecute(t *testing.T) {
	s, _, j, _ := dispositionFixture(t)
	ctx := credentialRootContext()
	require.NoError(t, os.RemoveAll(filepath.Join(s.cfg.Directory, "objects")))
	require.NoError(t, os.Mkdir(filepath.Join(s.cfg.Directory, "objects"), 0700))
	v, e := s.ReviewInterruptedCapture(ctx, j.ID, nil)
	require.NoError(t, e)
	in := retainInput(v)
	out, e := s.RetainInterruptedCapture(ctx, j.ID, in)
	require.NoError(t, e)
	require.Zero(t, out.Result.ObjectCount)
	_, e = s.Job(ctx, j.ID)
	require.NoError(t, e)
	require.NoError(t, s.saveJob(&j))
	in.RequestID = uuid.NewString()
	epoch, e := s.dispositionEpoch(ctx)
	require.NoError(t, e)
	r := dispositionRecord{InterruptionReceipt: InterruptionReceipt{RequestID: in.RequestID, JobID: j.ID, State: "registered", InputDigest: dispositionHash([]any{j.ID, in}), ExpectedVersion: in.ExpectedVersion, Action: in.Action, Reason: in.Reason, Operator: 11, CreatedAt: time.Now().UTC()}, Epoch: dispositionHash(epoch)}
	require.NoError(t, dispositionWrite(s.path("interruption-requests", in.RequestID), r))
	out, e = s.RetainInterruptedCapture(ctx, j.ID, in)
	require.NoError(t, e)
	require.Equal(t, "unconfirmed", out.State)
	current, e := s.Job(ctx, j.ID)
	require.NoError(t, e)
	require.True(t, current.InterruptionPending)
}

func TestInterruptionJobReceiptSurvivesMissingRequestFile(t *testing.T) {
	s, _, j, _ := dispositionFixture(t)
	ctx := credentialRootContext()
	v, e := s.ReviewInterruptedCapture(ctx, j.ID, nil)
	require.NoError(t, e)
	in := retainInput(v)
	_, e = s.RetainInterruptedCapture(ctx, j.ID, in)
	require.NoError(t, e)
	require.NoError(t, os.Remove(s.path("interruption-requests", in.RequestID)))
	out, e := s.InterruptionRequest(ctx, in.RequestID)
	require.NoError(t, e)
	require.Equal(t, "job_record", out.EvidenceSource)
	require.True(t, out.CurrentRecordMatches)
	_, e = s.RetainInterruptedCapture(ctx, j.ID, in)
	require.NoError(t, e)
	in.Reason = "不能复用已由任务保存的原请求编号"
	_, e = s.RetainInterruptedCapture(ctx, j.ID, in)
	require.Equal(t, 409, dto.StatusCode(e))
}
