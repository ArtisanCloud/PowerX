package snapshot_ops

import (
	"context"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestControlledCaptureAdmissionAndMaintenanceBoundary(t *testing.T) {
	s, _, _ := fixtureService(t)
	s.opts.RequirePolicyControl = true
	now := s.opts.Now()
	deadline := now.Add(45 * time.Minute)
	j := Snapshot{PolicyRevision: 1, PolicyFingerprint: s.PolicyFingerprint(), CreatedAt: now, AdmissionDeadline: &deadline}
	require.NoError(t, s.validateCaptureAdmission(&j))
	j.PolicyFingerprint = "changed"
	require.Equal(t, "SNAPSHOT_ADMISSION_UNCONFIRMED", dto.CodeOf(s.validateCaptureAdmission(&j)))
	j.PolicyFingerprint = s.PolicyFingerprint()
	expired := now.Add(-time.Minute)
	j.AdmissionDeadline = &expired
	require.Error(t, s.validateCaptureAdmission(&j))
	// 同一个程序的明确内部维护采集不依赖普通计划数据库，也不触发其自动清理。
	capture, e := s.Start(rootContext(), uuid.NewString())
	require.NoError(t, e)
	require.Equal(t, "maintenance", capture.Trigger)
	require.Empty(t, capture.PolicyID)
	require.Eventually(t, func() bool { j, e := s.Get(rootContext(), capture.ID); return e == nil && j.Status == "ready" }, 3*time.Second, 10*time.Millisecond)
	s.opts.DispatchCapture = func(context.Context, string) error { return nil }
	require.Equal(t, "SNAPSHOT_POLICY_EXECUTOR_UNSUPPORTED", dto.CodeOf(s.CheckPolicySources(rootContext())))
}

func TestControlledRetentionRejectsLinkedQuarantineParent(t *testing.T) {
	s, _, _ := fixtureService(t)
	external := t.TempDir()
	token := uuid.NewString()
	require.NoError(t, os.Mkdir(filepath.Join(external, token), 0700))
	sentinel := filepath.Join(external, token, "keep")
	require.NoError(t, os.WriteFile(sentinel, []byte("protected"), 0600))
	require.NoError(t, os.Symlink(external, filepath.Join(s.cfg.Directory, "retention")))
	_, e := s.retentionDirectory(true)
	require.Equal(t, "SNAPSHOT_RETENTION_PATH_UNSAFE", dto.CodeOf(e))
	require.Error(t, s.removeRetentionQuarantine(Snapshot{RetentionToken: token}))
	_, e = os.Stat(sentinel)
	require.NoError(t, e)
}
