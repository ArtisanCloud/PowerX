package archive_ops

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func unknownDispositionFixture(t *testing.T) (*Service, Policy, jobRecord, dispositionRecord) {
	t.Helper()
	s, p, j, _ := dispositionFixture(t)
	epoch, e := s.dispositionEpoch(credentialRootContext())
	require.NoError(t, e)
	input := RetainInterruptedInput{uuid.NewString(), dispositionHash("old preview"), dispositionAction, "中断前已登记，无法确认历史结果", true}
	r := dispositionRecord{InterruptionReceipt: InterruptionReceipt{RequestID: input.RequestID, JobID: j.ID, State: "registered", InputDigest: dispositionHash([]any{j.ID, input}), ExpectedVersion: input.ExpectedVersion, Action: input.Action, Reason: input.Reason, Operator: 1, CreatedAt: j.CreatedAt}, Epoch: dispositionHash(epoch)}
	require.NoError(t, dispositionWrite(s.path("interruption-requests", r.RequestID), r))
	return s, p, j, r
}
func closureInput(v InterruptionRequestReview) CloseInterruptionRequestInput {
	return CloseInterruptionRequestInput{uuid.NewString(), v.ReviewVersion, interruptionClosureAction, "人工核对后结束未知请求，保持原采集保护", true}
}
func TestInterruptionRequestClosureReadOnlyAndOriginalProtection(t *testing.T) {
	s, p, j, r := unknownDispositionFixture(t)
	ctx := credentialRootContext()
	beforeJob, e := os.ReadFile(s.path("jobs", j.ID))
	require.NoError(t, e)
	beforeRequest, e := os.ReadFile(s.path("interruption-requests", r.RequestID))
	require.NoError(t, e)
	beforeObjects := restartObjects(t, s.cfg.Directory)
	v, e := s.ReviewInterruptionRequest(ctx, r.RequestID)
	require.NoError(t, e)
	require.True(t, v.ReadOnly)
	require.True(t, v.InterruptionPending)
	require.True(t, v.RegisteredEpochMatches)
	after, e := os.ReadFile(s.path("interruption-requests", r.RequestID))
	require.NoError(t, e)
	require.Equal(t, beforeRequest, after)
	in := closureInput(v)
	out, e := s.CloseInterruptionRequest(ctx, r.RequestID, in)
	require.NoError(t, e)
	require.Equal(t, "closed_unknown", out.State)
	require.True(t, out.CurrentProtectionMatches)
	require.False(t, out.CurrentRecordMatches)
	require.Equal(t, r.InputDigest, out.InputDigest)
	require.Equal(t, r.Reason, out.Reason)
	require.Nil(t, out.Result)
	require.Equal(t, in.RequestID, out.Closure.RequestID)
	require.Equal(t, in.Reason, out.Closure.Reason)
	after, e = os.ReadFile(s.path("jobs", j.ID))
	require.NoError(t, e)
	require.Equal(t, beforeJob, after)
	require.Equal(t, beforeObjects, restartObjects(t, s.cfg.Directory))
	_, e = s.cleanupApproved(ctx, p.ID)
	require.NoError(t, e)
	require.Equal(t, beforeObjects, restartObjects(t, s.cfg.Directory), "request closure must not release interrupted global GC fence")
	replay, e := s.CloseInterruptionRequest(ctx, r.RequestID, in)
	require.NoError(t, e)
	require.Equal(t, out.Closure, replay.Closure)
	in.Reason += " changed"
	_, e = s.CloseInterruptionRequest(ctx, r.RequestID, in)
	require.Equal(t, 409, dto.StatusCode(e))
	original := RetainInterruptedInput{r.RequestID, r.ExpectedVersion, r.Action, r.Reason, true}
	historical, e := s.RetainInterruptedCapture(ctx, j.ID, original)
	require.NoError(t, e)
	require.Equal(t, "closed_unknown", historical.State)
	after, e = os.ReadFile(s.path("jobs", j.ID))
	require.NoError(t, e)
	require.Equal(t, beforeJob, after)
	// 新的显式内容核验与处置独立进行，原请求从不重放。
	content, e := s.ReviewInterruptedCapture(ctx, j.ID, nil)
	require.NoError(t, e)
	next, e := s.RetainInterruptedCapture(ctx, j.ID, retainInput(content))
	require.NoError(t, e)
	require.Equal(t, "retained", next.State)
	old, e := s.InterruptionRequest(ctx, r.RequestID)
	require.NoError(t, e)
	require.Equal(t, "closed_unknown", old.State)
	require.False(t, old.CurrentProtectionMatches)
}
func TestInterruptionRequestClosureStaleBusyAndRecovery(t *testing.T) {
	for _, scene := range []string{"epoch", "job", "policy", "request_lock", "policy_lock", "gc_lock", "cancel", "not_root"} {
		t.Run(scene, func(t *testing.T) {
			s, p, j, r := unknownDispositionFixture(t)
			ctx := credentialRootContext()
			v, e := s.ReviewInterruptionRequest(ctx, r.RequestID)
			require.NoError(t, e)
			switch scene {
			case "epoch":
				_, e = s.recoveryAnchor.Begin(ctx, uuid.NewString(), uuid.NewString())
				require.NoError(t, e)
			case "job":
				j.Error = "different current metadata"
				require.NoError(t, dispositionWrite(s.path("jobs", j.ID), j))
			case "policy":
				p.Name += " changed"
				_, e = s.SavePolicy(ctx, p)
				require.NoError(t, e)
			case "request_lock", "policy_lock", "gc_lock":
				name := "interruption-" + r.RequestID
				if scene == "policy_lock" {
					name = p.ID
				}
				if scene == "gc_lock" {
					name = "gc"
				}
				unlock, e := s.lock(ctx, name)
				require.NoError(t, e)
				defer unlock()
			case "cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "not_root":
				ctx = context.Background()
			}
			_, e = s.CloseInterruptionRequest(ctx, r.RequestID, closureInput(v))
			require.Error(t, e)
			current, e := s.readDisposition(r.RequestID)
			require.NoError(t, e)
			require.Equal(t, "registered", current.State)
			require.Nil(t, current.Closure)
			job, e := s.readJob(j.ID)
			require.NoError(t, e)
			require.True(t, job.InterruptionPending)
		})
	}
}
func TestInterruptionRequestClosureConcurrencyAndRollback(t *testing.T) {
	s, _, j, r := unknownDispositionFixture(t)
	ctx := credentialRootContext()
	_, e := s.recoveryAnchor.Begin(ctx, uuid.NewString(), uuid.NewString())
	require.NoError(t, e)
	v, e := s.ReviewInterruptionRequest(ctx, r.RequestID)
	require.NoError(t, e)
	require.False(t, v.RegisteredEpochMatches)
	in := closureInput(v)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, e := s.CloseInterruptionRequest(ctx, r.RequestID, in)
			if e == nil {
				require.Equal(t, "closed_unknown", out.State)
			} else {
				require.Equal(t, 409, dto.StatusCode(e))
			}
		}()
	}
	wg.Wait()
	out, e := s.InterruptionRequest(ctx, r.RequestID)
	require.NoError(t, e)
	require.True(t, out.CurrentProtectionMatches)
	raw, e := os.ReadFile(s.path("interruption-requests", r.RequestID))
	require.NoError(t, e)
	_, e = s.recoveryAnchor.Begin(ctx, uuid.NewString(), uuid.NewString())
	require.NoError(t, e)
	old, e := s.CloseInterruptionRequest(ctx, r.RequestID, in)
	require.NoError(t, e)
	require.False(t, old.CurrentProtectionMatches)
	after, e := os.ReadFile(s.path("interruption-requests", r.RequestID))
	require.NoError(t, e)
	require.Equal(t, raw, after, "replay must not renew current protection")
	require.NoError(t, os.Remove(s.path("jobs", j.ID)))
	missing, e := s.InterruptionRequest(ctx, r.RequestID)
	require.NoError(t, e)
	require.Equal(t, "closed_unknown", missing.State)
	require.False(t, missing.CurrentProtectionMatches)
}
func TestInterruptionRequestClosureCorruptionAndUUIDReuse(t *testing.T) {
	s, _, j, r := unknownDispositionFixture(t)
	ctx := credentialRootContext()
	v, e := s.ReviewInterruptionRequest(ctx, r.RequestID)
	require.NoError(t, e)
	in := closureInput(v)
	_, e = s.CloseInterruptionRequest(ctx, r.RequestID, in)
	require.NoError(t, e)
	content, e := s.ReviewInterruptedCapture(ctx, j.ID, nil)
	require.NoError(t, e)
	reuse := retainInput(content)
	reuse.RequestID = in.RequestID
	_, e = s.RetainInterruptedCapture(ctx, j.ID, reuse)
	require.Equal(t, 409, dto.StatusCode(e))
	current, e := s.readDisposition(r.RequestID)
	require.NoError(t, e)
	current.Closure.Reason += " tampered"
	require.NoError(t, dispositionWrite(s.path("interruption-requests", r.RequestID), current))
	_, e = s.InterruptionRequest(ctx, r.RequestID)
	require.Equal(t, 503, dto.StatusCode(e))
}

func TestInterruptionRequestClosureRechecksLegacyPauseAfterWaitingConfigLock(t *testing.T) {
	s, p, _, r := unknownDispositionFixture(t)
	ctx := credentialRootContext()
	v, e := s.ReviewInterruptionRequest(ctx, r.RequestID)
	require.NoError(t, e)
	s.mu.Lock()
	done := make(chan error, 1)
	go func() { _, e := s.CloseInterruptionRequest(ctx, r.RequestID, closureInput(v)); done <- e }()
	// 持有配置锁模拟 SavePolicy，等待处置已拿到原策略执行锁再写新配置。
	require.Eventually(t, func() bool {
		unlock, e := s.lock(ctx, p.ID)
		if e != nil {
			return true
		}
		unlock()
		return false
	}, time.Second, time.Millisecond)
	p.Enabled = true
	require.NoError(t, dispositionWrite(s.path("policies", p.ID), p))
	s.mu.Unlock()
	require.Equal(t, 409, dto.StatusCode(<-done))
	current, e := s.readDisposition(r.RequestID)
	require.NoError(t, e)
	require.Equal(t, "registered", current.State)
}
