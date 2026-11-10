package storagegov

import (
	"context"
	"encoding/json"
	recovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	archive "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestArchiveProbeNativePublishedScopeAndSingleConnection(t *testing.T) {
	target, ctrl, tgt, _ := targetNative(t)
	ctx := controlRoot()
	policy := &archivePolicyControl{target.service, target.archives}
	adoptNativeTarget(t, target, ctrl)
	adoptNativePolicy(t, policy, ctrl)
	snapshot, e := policy.ObserveProbeCommand(ctx, tgt.ID)
	require.NoError(t, e)
	require.NotNil(t, snapshot.Target)
	require.Empty(t, snapshot.Policies)
	pool, e := ctrl.repo.DB.DB()
	require.NoError(t, e)
	pool.SetMaxOpenConns(1)
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	called := false
	require.NoError(t, policy.GuardProbeCommand(bounded, tgt.ID, snapshot, func(archive.ProbeControlSnapshot) error { called = true; return nil }))
	require.True(t, called)
	// 正式保存未发布会阻止整个范围，不能从旧投影落回本地目录。
	current := snapshot.Target
	_, e = target.Save(ctx, archive.TargetChange{RequestID: uuid.NewString(), ExpectedRevision: snapshot.TargetRevision, ExpectedTargetRevision: current.Revision, TargetID: tgt.ID, Settings: archive.SettingsOfTarget(*current)})
	require.NoError(t, e)
	_, e = policy.ObserveProbeCommand(ctx, tgt.ID)
	require.Error(t, e)
	applyMonitoring(t, ctrl, ctrl.opts.Queue)
	called = false
	e = policy.GuardProbeCommand(ctx, tgt.ID, snapshot, func(archive.ProbeControlSnapshot) error { called = true; return nil })
	require.Error(t, e)
	require.False(t, called)
	fresh, e := policy.ObserveProbeCommand(ctx, tgt.ID)
	require.NoError(t, e)
	require.NotEqual(t, snapshot.Digest, fresh.Digest)
	// 完整权威快照中找不到目标，才是删除；正式读库失败不是删除。
	absent, e := policy.ObserveProbeCommand(ctx, uuid.NewString())
	require.NoError(t, e)
	require.Nil(t, absent.Target)
	offline, e := gorm.Open(postgres.Open(os.Getenv("POWERX_STORAGE_CONTROL_TEST_DSN")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	closed, e := offline.DB()
	require.NoError(t, e)
	require.NoError(t, closed.Close())
	previous := ctrl.repo.DB
	ctrl.repo.DB = offline
	defer func() { ctrl.repo.DB = previous }()
	_, e = policy.ObserveProbeCommand(ctx, tgt.ID)
	require.Error(t, e)
	called = false
	e = policy.GuardProbeCommand(ctx, tgt.ID, fresh, func(archive.ProbeControlSnapshot) error { called = true; return nil })
	require.Error(t, e)
	require.False(t, called)
}
func TestArchiveProbeNativeReferenceSetAndPublicationFence(t *testing.T) {
	policy, ctrl, p, dir := policyNative(t)
	ctx := controlRoot()
	target := &archiveTargetControl{policy.service, policy.archives}
	p.TargetID = uuid.NewString()
	raw, e := json.Marshal(p)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "policies", p.ID+".json"), raw, 0600))
	adoptNativeTarget(t, target, ctrl)
	adoptNativePolicy(t, policy, ctrl)
	// 暂停后的正式策略引用测试目标；通过正式控制链发布，不修改 JSON 投影。
	state, e := policy.Configuration(ctx)
	require.NoError(t, e)
	var active archive.Policy
	for _, x := range state.Policies {
		if x.ID == p.ID {
			active = x
		}
	}
	disabled := false
	_, e = policy.Save(ctx, archive.PolicyChange{RequestID: uuid.NewString(), ExpectedRevision: state.DesiredRevision, ExpectedPolicyRevision: active.Revision, PolicyID: p.ID, Action: "status", Enabled: &disabled, RemoteCleanupEnabled: &disabled, Confirm: true})
	require.NoError(t, e)
	applyMonitoring(t, ctrl, ctrl.opts.Queue)
	snapshot, e := policy.ObserveProbeCommand(ctx, p.TargetID)
	require.NoError(t, e)
	require.Len(t, snapshot.Policies, 1)
	require.False(t, snapshot.Policies[0].Enabled)
	// 未发布的新增/修改引用即阻止读写；配置恢复后也必须使用新摘要。
	require.NoError(t, ctrl.repo.DB.Model(&m.ControlConfiguration{}).Where("scope_id = ? AND domain = ?", ctrl.opts.Scope.ID(), string(ControlArchivePolicies)).Update("revision", snapshot.PolicyRevision+1).Error)
	called := false
	e = policy.GuardProbeCommand(ctx, p.TargetID, snapshot, func(archive.ProbeControlSnapshot) error { called = true; return nil })
	require.Error(t, e)
	require.False(t, called)
}

// 真实本机 ssh 已退出，仅隔离回放最终标记丢失；正式 SQL 权威贯穿人工确认。
func TestArchiveProbeNativeOriginalCommandProofWithAuthority(t *testing.T) {
	svc, ctrl, _, _ := monitoringNative(t)
	ctx := controlRoot()
	root := t.TempDir()
	key := filepath.Join(root, "key")
	require.NoError(t, exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key).Run())
	known := filepath.Join(root, "known")
	require.NoError(t, os.WriteFile(known, []byte{}, 0600))
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, e)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	cfg := archive.Config{Directory: filepath.Join(root, "archive"), Instance: "owned-probe-native", Credentials: map[string]archive.Credential{"owned": {KeyFile: key, KnownHostsFile: known}}}
	a := archive.NewService(context.Background(), cfg, archive.Options{})
	defer a.Close()
	target, e := a.SaveTarget(ctx, archive.Target{Name: "原SSH目标", Host: "127.0.0.1", Port: port, User: "archive", Directory: filepath.Join(root, "remote"), CredentialRef: "owned"})
	require.NoError(t, e)
	require.Error(t, a.TestTarget(ctx, target.ID))
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte("archive-command-probe/"+target.ID)).String()
	commandPath := filepath.Join(cfg.Directory, "command-executions", id+".json")
	raw, e := os.ReadFile(commandPath)
	require.NoError(t, e)
	var command map[string]any
	require.NoError(t, json.Unmarshal(raw, &command))
	require.Equal(t, float64(3), command["protocol"])
	require.NotNil(t, command["identity"])
	command["state"] = "running"
	raw, e = json.Marshal(command)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(commandPath, raw, 0600))
	maintenance := filepath.Join(root, "maintenance")
	require.NoError(t, os.Mkdir(maintenance, 0700))
	anchor, e := recovery.Initialize(maintenance, uuid.NewString())
	require.NoError(t, e)
	require.NoError(t, a.BindRecoveryAnchor(anchor))
	require.NoError(t, svc.BindArchiveControl(a))
	tc := &archiveTargetControl{svc, a}
	pc := &archivePolicyControl{svc, a}
	adoptNativeTarget(t, tc, ctrl)
	adoptNativePolicy(t, pc, ctrl)
	review, e := a.ReviewCommandDrain(ctx, id)
	require.NoError(t, e)
	require.True(t, review.Eligible)
	require.Equal(t, "changed", review.Probe.TargetState, "adoption changes revision; old scope remains explicit")
	input := archive.ConfirmCommandDrainInput{RequestID: uuid.NewString(), ExpectedVersion: review.ReviewVersion, ExecutionID: review.ExecutionID, CommandID: review.CommandID, Action: "confirm_local_command_drain", Reason: "正式权威核对原目标的本机命令退出", Confirm: true, TargetID: target.ID}
	state, e := tc.Configuration(ctx)
	require.NoError(t, e)
	current, e := tc.Resolve(ctx, target.ID)
	require.NoError(t, e)
	settings := archive.SettingsOfTarget(current)
	settings.Name = "更新后的目标"
	_, e = tc.Save(ctx, archive.TargetChange{RequestID: uuid.NewString(), ExpectedRevision: state.DesiredRevision, ExpectedTargetRevision: current.Revision, TargetID: target.ID, Settings: settings})
	require.NoError(t, e)
	_, e = a.ConfirmCommandDrain(ctx, id, input)
	require.Error(t, e)
	rejected, e := a.CommandConfirmationRequest(ctx, input.RequestID)
	require.NoError(t, e)
	require.Equal(t, "rejected", rejected.State)
	require.Equal(t, target.ID, rejected.TargetID)
	applyMonitoring(t, ctrl, ctrl.opts.Queue)
	review, e = a.ReviewCommandDrain(ctx, id)
	require.NoError(t, e)
	require.True(t, review.Eligible)
	input.RequestID = uuid.NewString()
	input.ExpectedVersion = review.ReviewVersion
	// 只有一个连接时也能在同一事务核对两份配置并落盘，不借用第二连接。
	pool, e := ctrl.repo.DB.DB()
	require.NoError(t, e)
	pool.SetMaxOpenConns(1)
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, e := a.ConfirmCommandDrain(bounded, id, input)
	require.NoError(t, e)
	require.True(t, out.CurrentRecordMatches)
	require.Empty(t, out.JobID)
	again, e := a.CommandConfirmationRequest(ctx, input.RequestID)
	require.NoError(t, e)
	require.Equal(t, out, again)
	after, e := os.ReadFile(commandPath)
	require.NoError(t, e)
	require.Equal(t, raw, after)
	current, e = tc.Resolve(ctx, target.ID)
	require.NoError(t, e)
	require.Nil(t, current.VerifiedAt)
	require.Equal(t, "更新后的目标", current.Name)
}
