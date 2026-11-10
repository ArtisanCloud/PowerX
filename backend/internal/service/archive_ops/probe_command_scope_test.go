package archive_ops

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	recovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// 真实 SSH 探测已退出；仅在隔离根模拟最终 running→drained 写入丢失。
func probeConfirmationFixture(t *testing.T) (*Service, Target, commandRecord) {
	t.Helper()
	credential, port := sshFixture(t)
	root := t.TempDir()
	s := NewService(context.Background(), Config{Directory: root, Instance: "owned-probe", Credentials: map[string]Credential{"owned": credential}}, Options{})
	t.Cleanup(s.Close)
	ctx := credentialRootContext()
	target, e := s.SaveTarget(ctx, Target{Name: "原目标", Host: "127.0.0.1", Port: port, User: "archive", Directory: filepath.Join(t.TempDir(), "remote"), CredentialRef: "owned"})
	require.NoError(t, e)
	anchorDir := filepath.Join(t.TempDir(), "maintenance")
	require.NoError(t, os.Mkdir(anchorDir, 0700))
	anchor, e := recovery.Initialize(anchorDir, uuid.NewString())
	require.NoError(t, e)
	require.NoError(t, s.BindRecoveryAnchor(anchor))
	require.NoError(t, s.TestTarget(ctx, target.ID))
	id := commandProbeID(target.ID)
	r, e := s.readCommandRecord(id)
	require.NoError(t, e)
	require.Equal(t, 3, r.Protocol)
	require.Empty(t, r.JobID)
	require.Equal(t, "ssh", r.Kind)
	require.NotNil(t, r.Identity)
	require.Equal(t, "drained", r.State)
	r.State = "running"
	require.NoError(t, writeJSON(s.path("command-executions", id), r))
	return s, target, r
}
func TestProbeConfirmationOriginalScopeReceiptAndNewExecution(t *testing.T) {
	s, target, r := probeConfirmationFixture(t)
	ctx := credentialRootContext()
	command, e := os.ReadFile(s.path("command-executions", r.ID))
	require.NoError(t, e)
	targetRaw, e := os.ReadFile(s.path("targets", target.ID))
	require.NoError(t, e)
	summary := s.commandOverview()
	require.Equal(t, 1, summary.Unconfirmed)
	require.Len(t, summary.Records, 1)
	require.Equal(t, target.ID, summary.Records[0].TargetID)
	require.Empty(t, summary.Jobs)
	review, e := s.ReviewCommandDrain(ctx, r.ID)
	require.NoError(t, e)
	require.True(t, review.Eligible)
	require.Equal(t, "probe", review.Subject)
	require.Equal(t, "unchanged", review.Probe.TargetState)
	require.Empty(t, review.JobID)
	in := commandConfirmationInput(review)
	receipt, e := s.ConfirmCommandDrain(ctx, r.ID, in)
	require.NoError(t, e)
	require.True(t, receipt.CurrentRecordMatches)
	require.Equal(t, target.ID, receipt.TargetID)
	require.Empty(t, receipt.JobID)
	require.NoError(t, s.requireCommandsDrained(false))
	after, e := os.ReadFile(s.path("command-executions", r.ID))
	require.NoError(t, e)
	require.Equal(t, command, after)
	after, e = os.ReadFile(s.path("targets", target.ID))
	require.NoError(t, e)
	require.Equal(t, targetRaw, after)
	again, e := s.ConfirmCommandDrain(ctx, r.ID, in)
	require.NoError(t, e)
	require.Equal(t, receipt, again)
	require.NoError(t, s.TestTarget(ctx, target.ID))
	current, e := s.readCommandRecord(r.ID)
	require.NoError(t, e)
	require.NotEqual(t, r.ExecutionID, current.ExecutionID)
	require.NotEqual(t, r.CommandID, current.CommandID)
	historical, e := s.CommandConfirmationRequest(ctx, in.RequestID)
	require.NoError(t, e)
	require.False(t, historical.CurrentRecordMatches)
}
func TestProbeConfirmationChangedDeletedAndScopeEcho(t *testing.T) {
	for _, scene := range []string{"changed", "deleted", "new_reference", "active_reference", "probe_lock", "policy_lock", "legacy", "scope_echo", "boot", "epoch"} {
		t.Run(scene, func(t *testing.T) {
			s, target, r := probeConfirmationFixture(t)
			ctx := credentialRootContext()
			review, e := s.ReviewCommandDrain(ctx, r.ID)
			require.NoError(t, e)
			in := commandConfirmationInput(review)
			var release func()
			switch scene {
			case "changed":
				target.Name = "新目标名称"
				_, e = s.SaveTarget(ctx, target)
				require.NoError(t, e)
			case "deleted":
				require.NoError(t, os.Remove(s.path("targets", target.ID))) // 仅构造旧目录已删除；正式权威删除另测。
			case "new_reference", "active_reference", "policy_lock":
				p := Policy{ID: uuid.NewString(), Name: "新引用", SourceID: "owned", TargetID: target.ID, Revision: 1, Enabled: scene == "active_reference"}
				require.NoError(t, writeJSON(s.path("policies", p.ID), p))
				if scene == "policy_lock" {
					release, e = s.lock(ctx, p.ID)
					require.NoError(t, e)
				}
			case "probe_lock":
				release, e = s.lock(ctx, "probe-target-"+target.ID)
				require.NoError(t, e)
			case "legacy":
				r.Protocol = 2
				r.Probe = nil
				require.NoError(t, writeJSON(s.path("command-executions", r.ID), r))
			case "scope_echo":
				in.TargetID = uuid.NewString()
			case "boot":
				r.Identity.Boot = dispositionHash("different boot")
				require.NoError(t, writeJSON(s.path("command-executions", r.ID), r))
			case "epoch":
				_, e = s.recoveryAnchor.Begin(ctx, uuid.NewString(), uuid.NewString())
				require.NoError(t, e)
			}
			if release != nil {
				defer release()
			}
			_, e = s.ConfirmCommandDrain(ctx, r.ID, in)
			require.Error(t, e)
			require.Error(t, s.requireCommandsDrained(false))
			if scene == "changed" || scene == "deleted" || scene == "new_reference" {
				fresh, e := s.ReviewCommandDrain(ctx, r.ID)
				require.NoError(t, e)
				require.True(t, fresh.Eligible)
				if scene != "new_reference" {
					require.Equal(t, scene, fresh.Probe.TargetState)
				} else {
					require.Len(t, fresh.Probe.Policies, 1)
				}
				result, e := s.ConfirmCommandDrain(ctx, r.ID, commandConfirmationInput(fresh))
				require.NoError(t, e)
				require.True(t, result.CurrentRecordMatches)
			}
			if scene == "legacy" {
				fresh, e := s.ReviewCommandDrain(ctx, r.ID)
				require.NoError(t, e)
				require.False(t, fresh.Eligible)
				require.Equal(t, "probe_scope_missing", fresh.Problem)
			}
		})
	}
}
func TestProbeConfirmationConcurrentRequestsAndLiveIdentity(t *testing.T) {
	s, _, r := probeConfirmationFixture(t)
	ctx := credentialRootContext()
	cmd := exec.Command("/bin/sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	identity, e := captureCommandIdentity(cmd.Process.Pid)
	require.NoError(t, e)
	r.PID = identity.PID
	r.PGID = identity.PGID
	r.Identity = identity
	require.NoError(t, writeJSON(s.path("command-executions", r.ID), r))
	v, e := s.ReviewCommandDrain(ctx, r.ID)
	require.NoError(t, e)
	require.False(t, v.Eligible)
	require.Equal(t, "group_not_drained", v.Problem)
	require.NoError(t, cmd.Process.Kill())
	require.Error(t, cmd.Wait())
	v, e = s.ReviewCommandDrain(ctx, r.ID)
	require.NoError(t, e)
	require.True(t, v.Eligible)
	in := commandConfirmationInput(v)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := s.ConfirmCommandDrain(ctx, r.ID, in); errs <- e }()
	}
	wg.Wait()
	close(errs)
	successes := 0
	for e := range errs {
		if e == nil {
			successes++
		} else {
			require.Equal(t, 409, dto.StatusCode(e))
		}
	}
	require.GreaterOrEqual(t, successes, 1)
	out, e := s.CommandConfirmationRequest(ctx, in.RequestID)
	require.NoError(t, e)
	require.True(t, out.CurrentRecordMatches)
}
func TestProbeTargetSerializedAndUnknownRecordNeverOverwritten(t *testing.T) {
	s, target, r := probeConfirmationFixture(t)
	ctx := credentialRootContext()
	_, e := s.initializeProbeCommand(target)
	require.Equal(t, "ARCHIVE_COMMAND_DRAIN_UNCONFIRMED", dto.CodeOf(e))
	unlock, e := s.lock(ctx, "probe-target-"+target.ID)
	require.NoError(t, e)
	require.Equal(t, "ARCHIVE_BUSY", dto.CodeOf(s.TestTarget(ctx, target.ID)))
	unlock()
	raw, e := os.ReadFile(s.path("command-executions", r.ID))
	require.NoError(t, e)
	var current commandRecord
	require.NoError(t, json.Unmarshal(raw, &current))
	require.Equal(t, r.ExecutionID, current.ExecutionID)
	require.WithinDuration(t, r.At, current.At, time.Second)
}
