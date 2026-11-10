package archive_ops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
)

type ownedTargetAuthority struct {
	target             Target
	rejectConfirmation bool
}

func (a *ownedTargetAuthority) ChangeDirectories() []string { return nil }
func (a *ownedTargetAuthority) Configuration(context.Context) (TargetConfiguration, error) {
	return TargetConfiguration{Authority: "database", State: "applied", Targets: []Target{a.target}}, nil
}
func (a *ownedTargetAuthority) Resolve(context.Context, string) (Target, error) { return a.target, nil }
func (a *ownedTargetAuthority) Confirm(_ context.Context, _ Target, save func(Target) error) error {
	if a.rejectConfirmation {
		return failure(409, "ARCHIVE_REVISION_CONFLICT", "changed during test")
	}
	return save(a.target)
}
func (a *ownedTargetAuthority) Save(context.Context, TargetChange) (TargetMutation, error) {
	panic("not used")
}
func (a *ownedTargetAuthority) Adopt(context.Context, TargetAdoption) (TargetMutation, error) {
	panic("not used")
}
func (a *ownedTargetAuthority) Request(context.Context, string) (TargetMutation, error) {
	panic("not used")
}
func TestArchiveTargetControlSeparateProofAndWorkerBinding(t *testing.T) {
	s, objects, target, _ := s3Fixture(t)
	original, e := os.ReadFile(s.path("targets", target.ID))
	require.NoError(t, e)
	target.Revision++
	target.ControlDigest = strings.Repeat("a", 64)
	authority := &ownedTargetAuthority{target: target}
	require.NoError(t, s.BindTargetAuthority(authority))
	ctx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true, UserID: 11, Scope: "access"})
	require.NoError(t, s.TestTarget(ctx, target.ID))
	active, e := s.target(target.ID)
	require.NoError(t, e)
	require.True(t, active.VerificationValid)
	unchanged, e := os.ReadFile(s.path("targets", target.ID))
	require.NoError(t, e)
	require.Equal(t, original, unchanged)
	objects.mu.Lock()
	require.Empty(t, objects.objects)
	objects.mu.Unlock()
	authority.target.Revision++
	authority.target.ControlDigest = strings.Repeat("b", 64)
	active, e = s.target(target.ID)
	require.NoError(t, e)
	require.False(t, active.VerificationValid, "old proof cannot verify edited configuration")
	authority.rejectConfirmation = true
	require.Error(t, s.TestTarget(ctx, target.ID))
	active, e = s.target(target.ID)
	require.NoError(t, e)
	require.False(t, active.VerificationValid)
	// 独立新版 worker 即使读取旧配置，也不能绕开私有接管标记。
	worker := NewService(context.Background(), s.cfg, Options{})
	defer worker.Close()
	_, e = worker.target(target.ID)
	require.Equal(t, "ARCHIVE_TARGET_CONTROL_UNAVAILABLE", dto.CodeOf(e))
	_, e = worker.SaveTarget(ctx, target)
	require.Equal(t, "ARCHIVE_TARGET_CONTROL_REQUIRED", dto.CodeOf(e))
	require.NoError(t, s.PublishRuntimeConfig())
	raw, e := os.ReadFile(filepath.Join(s.cfg.Directory, "runtime-config.yaml"))
	require.NoError(t, e)
	require.Contains(t, string(raw), "target_control_required: true")
}
