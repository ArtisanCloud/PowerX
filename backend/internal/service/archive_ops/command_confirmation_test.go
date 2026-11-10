package archive_ops

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	recovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// 控制夹具：采集真实内核身份后结束自有进程；记录模拟组件在 Wait 前退出，不能冒称真实 pg_dump 崩溃。
func commandConfirmationFixture(t *testing.T) (*Service, Policy, jobRecord, commandRecord) {
	t.Helper()
	s, p, j, _ := dispositionFixture(t)
	cmd := exec.Command("/bin/sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	identity, e := captureCommandIdentity(cmd.Process.Pid)
	require.NoError(t, e)
	t.Logf("owned confirmation identity pid=%d pgid=%d command=/bin/sleep 30 cwd=%s", identity.PID, identity.PGID, s.cfg.Directory)
	r := commandRecord{Protocol: 2, ID: j.ID, JobID: j.ID, ExecutionID: j.ExecutionID, CommandID: uuid.NewString(), Kind: "pg_dump", State: "running", PID: identity.PID, PGID: identity.PGID, Host: s.hostIdentity, At: time.Now().UTC(), Identity: identity}
	require.NoError(t, dispositionWrite(s.path("command-executions", r.ID), r))
	require.NoError(t, cmd.Process.Kill())
	require.Error(t, cmd.Wait())
	assertChildGone(t, commandChildProof{PID: r.PID, PGID: r.PGID})
	return s, p, j, r
}
func commandConfirmationInput(v CommandDrainReview) ConfirmCommandDrainInput {
	return ConfirmCommandDrainInput{uuid.NewString(), v.ReviewVersion, v.ExecutionID, v.CommandID, confirmCommandDrainAction, "人工核对原执行与系统启动后，确认本机组已退出", true, v.TargetID}
}
func TestCommandConfirmationImmutableSingleFileAndOriginalIdentity(t *testing.T) {
	s, _, j, r := commandConfirmationFixture(t)
	ctx := credentialRootContext()
	raw, e := os.ReadFile(s.path("command-executions", r.ID))
	require.NoError(t, e)
	job, e := os.ReadFile(s.path("jobs", j.ID))
	require.NoError(t, e)
	objects := restartObjects(t, s.cfg.Directory)
	v, e := s.ReviewCommandDrain(ctx, r.ID)
	require.NoError(t, e)
	require.True(t, v.Eligible)
	require.True(t, v.BootMatches)
	require.Equal(t, "absent", v.Leader)
	require.Equal(t, "absent", v.Group)
	after, e := os.ReadFile(s.path("command-executions", r.ID))
	require.NoError(t, e)
	require.Equal(t, raw, after)
	ids, e := s.records("command-confirmations")
	require.NoError(t, e)
	require.Empty(t, ids)
	in := commandConfirmationInput(v)
	out, e := s.ConfirmCommandDrain(ctx, r.ID, in)
	require.NoError(t, e)
	require.True(t, out.CurrentRecordMatches)
	require.Equal(t, "drained", s.jobCommandState(j))
	require.NoError(t, s.requireCommandsDrained(false))
	original, e := os.ReadFile(s.path("jobs", j.ID))
	require.NoError(t, e)
	require.Equal(t, job, original)
	after, e = os.ReadFile(s.path("command-executions", r.ID))
	require.NoError(t, e)
	require.Equal(t, raw, after)
	require.Equal(t, objects, restartObjects(t, s.cfg.Directory))
	receipt, e := s.CommandConfirmationRequest(ctx, in.RequestID)
	require.NoError(t, e)
	require.Equal(t, out, receipt)
	replay, e := s.ConfirmCommandDrain(ctx, r.ID, in)
	require.NoError(t, e)
	require.Equal(t, out, replay)
	other := in
	other.Reason += " changed"
	_, e = s.ConfirmCommandDrain(ctx, r.ID, other)
	require.Equal(t, 409, dto.StatusCode(e))
	other = in
	other.RequestID = uuid.NewString()
	_, e = s.ConfirmCommandDrain(ctx, r.ID, other)
	require.Equal(t, 409, dto.StatusCode(e))
	content, e := s.ReviewInterruptedCapture(ctx, j.ID, nil)
	require.NoError(t, e)
	retained, e := s.RetainInterruptedCapture(ctx, j.ID, retainInput(content))
	require.NoError(t, e)
	require.Equal(t, "retained", retained.State)
	receipt, e = s.CommandConfirmationRequest(ctx, in.RequestID)
	require.NoError(t, e)
	require.True(t, receipt.CurrentRecordMatches, "only original execution proof, not a whole-job completion claim")
}
func TestCommandConfirmationStaleEpochPolicyJobAndMissingEvidence(t *testing.T) {
	for _, scene := range []string{"epoch", "job", "policy", "boot", "identity_missing", "legacy", "command_lock", "policy_lock", "missing_file", "not_root"} {
		t.Run(scene, func(t *testing.T) {
			s, p, j, r := commandConfirmationFixture(t)
			ctx := credentialRootContext()
			v, e := s.ReviewCommandDrain(ctx, r.ID)
			require.NoError(t, e)
			switch scene {
			case "epoch":
				_, e = s.recoveryAnchor.Begin(ctx, uuid.NewString(), uuid.NewString())
				require.NoError(t, e)
			case "job":
				j.Error = "new context"
				require.NoError(t, s.saveJob(&j))
			case "policy":
				p.Name += " changed"
				_, e = s.SavePolicy(ctx, p)
				require.NoError(t, e)
			case "boot":
				r.Identity.Boot = strings.Repeat("c", 64)
				require.NoError(t, dispositionWrite(s.path("command-executions", r.ID), r))
			case "identity_missing":
				r.Identity = nil
				require.NoError(t, dispositionWrite(s.path("command-executions", r.ID), r))
			case "legacy":
				r.Protocol = 1
				r.Identity = nil
				require.NoError(t, dispositionWrite(s.path("command-executions", r.ID), r))
			case "command_lock", "policy_lock":
				key := "command-" + r.ID
				if scene == "policy_lock" {
					key = p.ID
				}
				unlock, e := s.lock(ctx, key)
				require.NoError(t, e)
				defer unlock()
			case "missing_file":
				require.NoError(t, os.Remove(s.path("command-executions", r.ID)))
			case "not_root":
				ctx = context.Background()
			}
			input := commandConfirmationInput(v)
			_, e = s.ConfirmCommandDrain(ctx, r.ID, input)
			require.Error(t, e)
			if scene != "not_root" {
				rejected, e := s.CommandConfirmationRequest(credentialRootContext(), input.RequestID)
				require.NoError(t, e)
				require.Equal(t, "rejected", rejected.State)
				require.False(t, rejected.CurrentRecordMatches)
				input.Reason += " changed"
				_, e = s.ConfirmCommandDrain(credentialRootContext(), r.ID, input)
				require.Equal(t, 409, dto.StatusCode(e))
			}
			ids, e := s.records("command-confirmations")
			require.NoError(t, e)
			require.Empty(t, ids)
			current, e := s.readJob(j.ID)
			require.NoError(t, e)
			require.True(t, current.InterruptionPending)
		})
	}
}
func TestCommandConfirmationConcurrentAndRecoveryReplayNeverRestoresCurrentProof(t *testing.T) {
	s, _, j, r := commandConfirmationFixture(t)
	ctx := credentialRootContext()
	v, e := s.ReviewCommandDrain(ctx, r.ID)
	require.NoError(t, e)
	in := commandConfirmationInput(v)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, e := s.ConfirmCommandDrain(ctx, r.ID, in)
			if e == nil {
				require.True(t, out.CurrentRecordMatches)
			} else {
				require.Equal(t, 409, dto.StatusCode(e))
			}
		}()
	}
	wg.Wait()
	raw, e := os.ReadFile(s.path("command-confirmations", r.CommandID))
	require.NoError(t, e)
	_, e = s.recoveryAnchor.Begin(ctx, uuid.NewString(), uuid.NewString())
	require.NoError(t, e)
	receipt, e := s.ConfirmCommandDrain(ctx, r.ID, in)
	require.NoError(t, e)
	require.False(t, receipt.CurrentRecordMatches)
	require.Equal(t, "unconfirmed", s.jobCommandState(j))
	after, e := os.ReadFile(s.path("command-confirmations", r.CommandID))
	require.NoError(t, e)
	require.Equal(t, raw, after)
	require.NoError(t, os.Remove(s.path("command-executions", r.ID)))
	receipt, e = s.CommandConfirmationRequest(ctx, in.RequestID)
	require.NoError(t, e)
	require.False(t, receipt.CurrentRecordMatches)
}
func TestCommandConfirmationRefusesLiveOrReusedPIDWithoutSignals(t *testing.T) {
	s, _, j, r := commandConfirmationFixture(t)
	ctx := credentialRootContext()
	cmd := exec.Command("/bin/sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	live, e := captureCommandIdentity(cmd.Process.Pid)
	require.NoError(t, e)
	t.Logf("owned live/reuse comparison pid=%d pgid=%d command=/bin/sleep 30 cwd=%s", live.PID, live.PGID, s.cfg.Directory)
	r.PID = live.PID
	r.PGID = live.PGID
	r.Identity = live
	require.NoError(t, dispositionWrite(s.path("command-executions", r.ID), r))
	v, e := s.ReviewCommandDrain(ctx, r.ID)
	require.NoError(t, e)
	require.False(t, v.Eligible)
	require.Equal(t, "matching", v.Leader)
	_, e = s.ConfirmCommandDrain(ctx, r.ID, commandConfirmationInput(v))
	require.Equal(t, 409, dto.StatusCode(e))
	require.NoError(t, syscall.Kill(live.PID, 0))
	r.Identity.Started = "1" // 控制夹具模拟同一 PID 的不同内核出生身份，不声称强制了内核 PID 分配。
	require.NoError(t, dispositionWrite(s.path("command-executions", r.ID), r))
	v, e = s.ReviewCommandDrain(ctx, r.ID)
	require.NoError(t, e)
	require.False(t, v.Eligible)
	require.Equal(t, "identity_mismatch", v.Problem)
	_, e = s.ConfirmCommandDrain(ctx, r.ID, commandConfirmationInput(v))
	require.Equal(t, 409, dto.StatusCode(e))
	require.NoError(t, syscall.Kill(live.PID, 0))
	original, e := s.readJob(j.ID)
	require.NoError(t, e)
	require.True(t, original.InterruptionPending)
}
func TestCommandConfirmationProofCorruptionAndNewCommandCannotInherit(t *testing.T) {
	s, _, j, r := commandConfirmationFixture(t)
	ctx := credentialRootContext()
	v, e := s.ReviewCommandDrain(ctx, r.ID)
	require.NoError(t, e)
	in := commandConfirmationInput(v)
	_, e = s.ConfirmCommandDrain(ctx, r.ID, in)
	require.NoError(t, e)
	original := r
	r.CommandID = uuid.NewString()
	require.NoError(t, dispositionWrite(s.path("command-executions", r.ID), r))
	require.Equal(t, "unconfirmed", s.jobCommandState(j))
	receipt, e := s.CommandConfirmationRequest(ctx, in.RequestID)
	require.NoError(t, e)
	require.False(t, receipt.CurrentRecordMatches)
	require.NoError(t, dispositionWrite(s.path("command-executions", r.ID), original))
	require.NoError(t, os.WriteFile(filepath.Join(s.cfg.Directory, "command-confirmations", original.CommandID+".json"), []byte("broken"), 0600))
	require.Equal(t, "unconfirmed", s.jobCommandState(j))
	_, e = s.CommandConfirmationRequest(ctx, in.RequestID)
	require.Equal(t, 503, dto.StatusCode(e))
}

func TestCommandConfirmationRealOrphanAfterSIGKILLRequiresExternalGroupExit(t *testing.T) {
	s, _, p := fixture(t)
	root, e := os.MkdirTemp("", "powerx-command-drain-confirm-")
	require.NoError(t, e)
	defer os.RemoveAll(root)
	raw, e := marshalConfig(s.cfg)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.yaml"), raw, 0600))
	id := uuid.NewString()
	cmd := ownedCommand(root, "crash-parent")
	cmd.Env = append(cmd.Env, "POWERX_COMMAND_JOB="+id, "POWERX_COMMAND_POLICY="+p.ID)
	require.NoError(t, cmd.Start())
	leader := readChild(t, root, "leader.json")
	child := readChild(t, root, "descendant.json")
	t.Logf("owned confirmation parent=%d leader=%d child=%d pgid=%d port=%d cwd=%s command=%q", cmd.Process.Pid, leader.PID, child.PID, leader.PGID, child.Port, root, cmd.Args)
	defer func() {
		killOwnedTestCommandGroup(root, leader)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		assertChildGone(t, leader)
		assertChildGone(t, child)
	}()
	require.NoError(t, cmd.Process.Kill())
	require.Error(t, cmd.Wait())
	peer := NewService(context.Background(), s.cfg, Options{})
	defer peer.Close()
	anchorRoot := filepath.Join(t.TempDir(), "maintenance")
	require.NoError(t, os.Mkdir(anchorRoot, 0700))
	anchor, e := recovery.Initialize(anchorRoot, uuid.NewString())
	require.NoError(t, e)
	require.NoError(t, peer.BindRecoveryAnchor(anchor))
	ctx := credentialRootContext()
	review, e := peer.ReviewCommandDrain(ctx, id)
	require.NoError(t, e)
	require.False(t, review.Eligible)
	require.Equal(t, "matching", review.Leader)
	_, e = peer.ConfirmCommandDrain(ctx, id, commandConfirmationInput(review))
	require.Equal(t, 409, dto.StatusCode(e))
	require.NoError(t, syscall.Kill(leader.PID, 0))
	killOwnedTestCommandGroup(root, leader)
	assertChildGone(t, leader)
	assertChildGone(t, child)
	review, e = peer.ReviewCommandDrain(ctx, id)
	require.NoError(t, e)
	require.True(t, review.Eligible)
	receipt, e := peer.ConfirmCommandDrain(ctx, id, commandConfirmationInput(review))
	require.NoError(t, e)
	require.True(t, receipt.CurrentRecordMatches)
	original, e := peer.Job(ctx, id)
	require.NoError(t, e)
	require.Equal(t, "drained", original.CommandDrainState)
	require.True(t, original.InterruptionPending)
	jobs, e := peer.records("jobs")
	require.NoError(t, e)
	require.Equal(t, []string{id}, jobs)
}
