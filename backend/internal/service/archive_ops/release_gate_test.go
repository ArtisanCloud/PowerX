package archive_ops

import (
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestCoreReleaseBlocksRemoteBeforeSideEffectsAndKeepsLocalExports(t *testing.T) {
	s, _, p := fixture(t)
	s.cfg.RemoteExecutionBlocked = true
	ctx := archiveRoot()
	target := Target{ID: uuid.NewString(), Driver: "s3"}
	remote := p
	remote.TargetID = target.ID
	remote.Enabled = true
	_, e := s.StartOneTime(ctx, "media", uuid.NewString(), "owned", "")
	require.NoError(t, e)
	// 与本地任务互斥无关，发布范围门槛在任何网络、凭据和远端命令前拒绝。
	_, e = s.remote(ctx, target, "test", uuid.NewString(), "-", nil)
	require.Equal(t, "ARCHIVE_RELEASE_NOT_ACCEPTED", dto.CodeOf(e))
	e = s.rsync(ctx, target, "unused", "unused", false, uuid.NewString())
	require.Equal(t, 423, dto.StatusCode(e))
	_, e = s.s3Client(target)
	require.Equal(t, 423, dto.StatusCode(e))
	e = s.TestTarget(ctx, target.ID)
	require.Equal(t, 423, dto.StatusCode(e))
	_, e = s.PreparePolicy(ctx, remote)
	require.Equal(t, 423, dto.StatusCode(e))
	_, e = s.SavePolicy(ctx, remote)
	require.Equal(t, 423, dto.StatusCode(e))
	before, e := s.records("jobs")
	require.NoError(t, e)
	_, e = s.startPolicyAtRevision(ctx, remote, uuid.NewString(), "owned", "", false, 0)
	require.Equal(t, 423, dto.StatusCode(e))
	after, e := s.records("jobs")
	require.NoError(t, e)
	require.Equal(t, before, after)
	require.NoError(t, s.transferAllowed(ctx, jobRecord{}))
	require.Equal(t, 423, dto.StatusCode(s.transferAllowed(ctx, jobRecord{Job: Job{Target: &target}})))
	view, e := s.Overview(ctx)
	require.NoError(t, e)
	require.False(t, view.RemoteExecution.Enabled)
	require.Equal(t, "ARCHIVE_RELEASE_NOT_ACCEPTED", view.RemoteExecution.Problem)
	// 发布阻塞会随正式运行配置下发；旧 worker 不能因未传 Service Options 丢失保护。
	raw, e := marshalConfig(s.cfg)
	require.NoError(t, e)
	path := s.cfg.Directory + "/release-config.yaml"
	require.NoError(t, os.WriteFile(path, raw, 0600))
	decoded, e := LoadConfig(path)
	require.NoError(t, e)
	require.True(t, decoded.RemoteExecutionBlocked)
}
