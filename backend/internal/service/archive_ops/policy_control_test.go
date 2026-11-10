package archive_ops

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestArchivePolicyWriteFailurePreservesOnlySafeControlCode(t *testing.T) {
	err := archiveWriteError(failure(409, "ARCHIVE_POLICY_CHANGED", "策略版本变化"), "归档清单发布失败")
	require.Equal(t, "ARCHIVE_POLICY_CHANGED", dto.CodeOf(err))
	require.Contains(t, err.Error(), "ARCHIVE_POLICY_CHANGED")
	err = archiveWriteError(errors.New("external endpoint with private credential"), "归档清单发布失败")
	require.EqualError(t, err, "归档清单发布失败")
	require.Equal(t, "ARCHIVE_WRITE_INTERRUPTED", dto.CodeOf(archiveWriteError(context.Canceled, "归档清单发布失败")))
}

type ownedPolicyAuthority struct{ policy Policy }

func (a *ownedPolicyAuthority) Configuration(context.Context) (PolicyConfiguration, error) {
	return PolicyConfiguration{Authority: "database", State: "applied", Policies: []Policy{a.policy}}, nil
}
func (a *ownedPolicyAuthority) Resolve(context.Context, string) (Policy, error) { return a.policy, nil }
func (a *ownedPolicyAuthority) Guard(_ context.Context, p Policy, op string, fn func([]Policy, *Target) error) error {
	if p.ID != "" && !a.policy.Enabled && !a.policy.RemoteCleanupEnabled {
		return failure(409, "ARCHIVE_POLICY_DISABLED", "paused")
	}
	return fn([]Policy{a.policy}, nil)
}
func (a *ownedPolicyAuthority) Save(context.Context, PolicyChange) (PolicyMutation, error) {
	panic("not used")
}
func (a *ownedPolicyAuthority) Adopt(context.Context, PolicyAdoption) (PolicyMutation, error) {
	panic("not used")
}
func (a *ownedPolicyAuthority) Request(context.Context, string) (PolicyMutation, error) {
	panic("not used")
}
func TestArchivePolicyLocalRetentionPreservesProtectedPendingAndPausedObjects(t *testing.T) {
	s, _, _, p := s3Fixture(t)
	p.Enabled = true
	p.RetentionDays = 1
	p.RetentionCount = 1
	p.ControlDigest = "owned-policy"
	authority := &ownedPolicyAuthority{p}
	require.NoError(t, s.BindPolicyAuthority(authority))
	makeJob := func(char, status string, at time.Time, protected bool) jobRecord {
		hash := strings.Repeat(char, 64)
		path := s.objectPath(hash)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
		require.NoError(t, os.WriteFile(path, []byte("owned"), 0600))
		j := jobRecord{Job: Job{ID: uuid.NewString(), PolicyID: p.ID, Status: status, RemoteState: "success", CreatedAt: at, Protected: protected, Files: []Entry{{SHA256: hash, Path: "owned.log", Size: 5}}}}
		require.NoError(t, s.saveJob(&j))
		return j
	}
	makeJob("a", "success", time.Now(), false)
	old := makeJob("b", "success", time.Now().Add(-72*time.Hour), false)
	protected := makeJob("c", "success", time.Now().Add(-72*time.Hour), true)
	pending := makeJob("d", "expiry_pending", time.Now().Add(-72*time.Hour), false)
	n, e := s.cleanup(context.Background(), p.ID)
	require.NoError(t, e)
	require.Equal(t, 1, n)
	_, e = os.Stat(s.objectPath(old.Files[0].SHA256))
	require.True(t, os.IsNotExist(e))
	for _, j := range []jobRecord{protected, pending} {
		_, e = os.Stat(s.objectPath(j.Files[0].SHA256))
		require.NoError(t, e)
	}
	paused := makeJob("e", "pruned", time.Now().Add(-72*time.Hour), false)
	authority.policy.Enabled = false
	n, e = s.cleanup(context.Background(), p.ID)
	require.NoError(t, e)
	require.Zero(t, n)
	_, e = s.cleanup(context.Background(), "")
	require.NoError(t, e)
	_, e = os.Stat(s.objectPath(paused.Files[0].SHA256))
	require.NoError(t, e, "another cleanup cannot purge a paused policy's retained references")
	worker := NewService(context.Background(), s.cfg, Options{})
	defer worker.Close()
	_, e = worker.SavePolicy(archiveRoot(), p)
	require.Equal(t, "ARCHIVE_POLICY_CONTROL_REQUIRED", dto.CodeOf(e))
	_, e = worker.policy(context.Background(), p.ID)
	require.Equal(t, "ARCHIVE_POLICY_CONTROL_UNAVAILABLE", dto.CodeOf(e))
}

func TestArchivePolicyTargetNamespaceChangePreservesHistoricalArchives(t *testing.T) {
	s, _, target, p := s3Fixture(t)
	require.NoError(t, s.TestTarget(archiveRoot(), target.ID))
	p.Enabled = true
	var e error
	p, e = s.SavePolicy(archiveRoot(), p)
	require.NoError(t, e)
	first := captureTest(t, s, p)
	require.Equal(t, "success", first.RemoteState)
	captureTest(t, s, p)
	target, e = s.target(target.ID)
	require.NoError(t, e)
	target.Prefix = "hosts/new-namespace"
	target, e = s.SaveTarget(archiveRoot(), target)
	require.NoError(t, e)
	require.NoError(t, s.TestTarget(archiveRoot(), target.ID))
	target, e = s.target(target.ID)
	require.NoError(t, e)
	p.RemoteCleanupEnabled = true
	p, e = s.SavePolicy(archiveRoot(), p)
	require.NoError(t, e)
	s.opts.RetentionNow = func() time.Time { return time.Now().Add(181 * 24 * time.Hour) }
	plan, e := s.buildRemoteGC(context.Background(), p, target)
	require.NoError(t, e)
	require.Empty(t, plan.Jobs)
	require.Empty(t, plan.Versions)
	require.Equal(t, 2, plan.Protected["historical_target_configuration"])
	s.executeRemoteGC(context.Background(), &plan)
	out, e := s.readJob(first.ID)
	require.NoError(t, e)
	require.Equal(t, "success", out.RemoteState)
}
