package snapshot_ops

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestMultiplePoliciesPreserveLegacyConfigurationAndAnchor(t *testing.T) {
	s, _, _ := fixtureService(t)
	anchor := time.Date(2026, 10, 17, 1, 0, 0, 0, time.UTC)
	legacy := policyRecord{Policy: Policy{Enabled: true, IntervalMinutes: 360, RetentionDays: 3, RetentionCount: 12}, Anchor: anchor}
	file := filepath.Join(s.cfg.Directory, "policy.json")
	require.NoError(t, atomicJSON(file, legacy))
	before, e := os.ReadFile(file)
	require.NoError(t, e)
	overview, e := s.Overview(rootContext())
	require.NoError(t, e)
	require.Len(t, overview.Policies, 1)
	require.Equal(t, legacy.Policy, overview.Policy)
	require.Equal(t, anchor.Add(6*time.Hour), *overview.NextRunAt)
	require.Equal(t, legacyPolicyID, overview.DefaultPolicyID)
	second, e := s.PutPolicy(rootContext(), "", "Daily", defaultPolicy())
	require.NoError(t, e)
	require.False(t, second.Default)
	require.False(t, second.Enabled)
	state, e := s.policyState()
	require.NoError(t, e)
	require.Equal(t, legacy.Policy, state.current().Policy)
	require.Equal(t, anchor, state.current().Anchor)
	after, e := os.ReadFile(file)
	require.NoError(t, e)
	require.Equal(t, before, after, "the original single-policy file is retained unchanged")
	info, e := os.Stat(filepath.Join(s.cfg.Directory, "policies.json"))
	require.NoError(t, e)
	require.Zero(t, info.Mode().Perm()&0077)
}

func TestSelectSystemDefaultLeavesSchedulesOff(t *testing.T) {
	s, backend, _ := fixtureService(t)
	p := defaultPolicy()
	p.Enabled = true
	a, e := s.PutPolicy(rootContext(), "", "Daily", p)
	require.NoError(t, e)
	p.Enabled, p.IntervalMinutes = false, 60
	b, e := s.PutPolicy(rootContext(), "", "Hourly", p)
	require.NoError(t, e)
	selected, e := s.SetDefaultPolicy(rootContext(), b.ID)
	require.NoError(t, e)
	require.True(t, selected.Default)
	require.False(t, selected.Enabled)
	require.Equal(t, p, selected.Policy)
	overview, e := s.Overview(rootContext())
	require.NoError(t, e)
	require.Equal(t, b.ID, overview.DefaultPolicyID)
	require.Nil(t, overview.NextRunAt)
	for _, row := range overview.Policies {
		require.False(t, row.Enabled)
		require.Nil(t, row.NextRunAt)
	}
	require.NoError(t, s.Tick(rootContext()))
	require.Zero(t, backend.captures.Load(), "choosing a policy must not schedule or execute a backup")
	_, e = s.SetDefaultPolicy(rootContext(), uuid.NewString())
	require.Equal(t, "SNAPSHOT_POLICY_NOT_FOUND", dto.CodeOf(e))
	_, e = s.SetDefaultPolicy(context.Background(), a.ID)
	require.Equal(t, "SNAPSHOT_ROOT_REQUIRED", dto.CodeOf(e))
}

func TestMultiplePoliciesSelectOneDefaultAndRetainDisabledConfigurations(t *testing.T) {
	s, _, _ := fixtureService(t)
	overview, e := s.Overview(rootContext())
	require.NoError(t, e)
	require.Empty(t, overview.Policies)
	p := defaultPolicy()
	a, e := s.PutPolicy(rootContext(), "", "Daily", p)
	require.NoError(t, e)
	require.True(t, a.Default)
	b, e := s.PutPolicy(rootContext(), "", "Before upgrade", p)
	require.NoError(t, e)
	_, e = s.SetPolicyEnabled(rootContext(), a.ID, true)
	require.NoError(t, e)
	_, e = s.SetPolicyEnabled(rootContext(), b.ID, true)
	require.NoError(t, e)
	state, e := s.policyState()
	require.NoError(t, e)
	require.Equal(t, b.ID, state.DefaultID)
	require.False(t, state.Policies[0].Enabled)
	require.True(t, state.Policies[1].Enabled)
	_, e = s.SetPolicyEnabled(rootContext(), b.ID, false)
	require.NoError(t, e)
	state, e = s.policyState()
	require.NoError(t, e)
	require.Equal(t, b.ID, state.DefaultID)
	require.Len(t, state.Policies, 2)
	require.False(t, state.current().Enabled)
	_, e = s.PutPolicy(rootContext(), "", "DAILY", p)
	require.Equal(t, "SNAPSHOT_POLICY_NAME_EXISTS", dto.CodeOf(e))
	_, e = s.PutPolicy(rootContext(), uuid.NewString(), "Missing", p)
	require.Equal(t, "SNAPSHOT_POLICY_NOT_FOUND", dto.CodeOf(e))
	_, e = s.SetPolicyEnabled(context.Background(), a.ID, true)
	require.Equal(t, "SNAPSHOT_ROOT_REQUIRED", dto.CodeOf(e))
	before, e := os.ReadFile(filepath.Join(s.cfg.Directory, "policies.json"))
	require.NoError(t, e)
	s.opts.Audit = func(context.Context, AuditEvent) error { return errors.New("audit unavailable") }
	_, e = s.SetPolicyEnabled(rootContext(), a.ID, true)
	require.ErrorContains(t, e, "audit unavailable")
	after, e := os.ReadFile(filepath.Join(s.cfg.Directory, "policies.json"))
	require.NoError(t, e)
	require.Equal(t, before, after, "failed activation must not persist a partial switch")
}

func TestConcurrentPolicyActivationKeepsOneEnabledDefault(t *testing.T) {
	s, _, _ := fixtureService(t)
	a, e := s.PutPolicy(rootContext(), "", "A", defaultPolicy())
	require.NoError(t, e)
	b, e := s.PutPolicy(rootContext(), "", "B", defaultPolicy())
	require.NoError(t, e)
	var wg sync.WaitGroup
	errorsOut := make(chan error, 60)
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := a.ID
			if i%2 == 0 {
				id = b.ID
			}
			_, e := s.SetPolicyEnabled(rootContext(), id, true)
			if e != nil && dto.CodeOf(e) != "SNAPSHOT_BUSY" {
				errorsOut <- e
			}
			_, e = s.policyState() // 严格读取会拒绝任何双启用或默认引用不完整的状态。
			if e != nil {
				errorsOut <- e
			}
		}(i)
	}
	wg.Wait()
	close(errorsOut)
	for e := range errorsOut {
		require.NoError(t, e)
	}
	state, e := s.policyState()
	require.NoError(t, e)
	enabled := 0
	for _, p := range state.Policies {
		if p.Enabled {
			enabled++
			require.Equal(t, state.DefaultID, p.ID)
		}
	}
	require.Equal(t, 1, enabled)
}

func TestSchedulerUsesSelectedPolicyAndTagsSnapshots(t *testing.T) {
	s, _, _ := fixtureService(t)
	now := time.Date(2026, 10, 17, 1, 0, 0, 0, time.UTC)
	s.opts.Now = func() time.Time { return now }
	a, e := s.PutPolicy(rootContext(), "", "Daily", defaultPolicy())
	require.NoError(t, e)
	b, e := s.PutPolicy(rootContext(), "", "Hourly", Policy{IntervalMinutes: 60, RetentionDays: 7, RetentionCount: 7})
	require.NoError(t, e)
	_, e = s.SetPolicyEnabled(rootContext(), b.ID, true)
	require.NoError(t, e)
	// 另一套策略的历史任务不能推迟所选策略的执行时间。
	old := Snapshot{ID: uuid.NewString(), PolicyID: a.ID, CreatedAt: now.Add(24 * time.Hour), Status: "failed"}
	require.NoError(t, atomicJSON(s.record("snapshots", old.ID), old))
	require.NoError(t, atomicJSON(s.record("requests", old.ID), map[string]string{"hash": digest([]byte("old fixture request"))}))
	overview, e := s.Overview(rootContext())
	require.NoError(t, e)
	require.Equal(t, now.Add(time.Hour), *overview.NextRunAt)
	require.NoError(t, s.Tick(context.Background()))
	now = now.Add(61 * time.Minute)
	require.NoError(t, s.Tick(context.Background()))
	waitIdle(t, s)
	overview, e = s.Overview(rootContext())
	require.NoError(t, e)
	require.Len(t, overview.Snapshots, 2)
	for _, snapshot := range overview.Snapshots {
		if snapshot.ID == old.ID {
			continue
		}
		require.Equal(t, "ready", snapshot.Status)
		require.Equal(t, b.ID, snapshot.PolicyID)
		require.Equal(t, b.Name, snapshot.PolicyName)
		require.Equal(t, "scheduled", snapshot.Trigger)
	}
}

func TestRetentionDoesNotApplyNewDefaultRulesToOtherPolicies(t *testing.T) {
	s, _, _ := fixtureService(t)
	a, e := s.PutPolicy(rootContext(), "", "Keep two", Policy{IntervalMinutes: 1440, RetentionDays: 7, RetentionCount: 2})
	require.NoError(t, e)
	b, e := s.PutPolicy(rootContext(), "", "Keep one", Policy{IntervalMinutes: 1440, RetentionDays: 7, RetentionCount: 1})
	require.NoError(t, e)
	_, e = s.SetPolicyEnabled(rootContext(), b.ID, true)
	require.NoError(t, e)
	seed := func(policyID string, hours int) string {
		id := uuid.NewString()
		j := Snapshot{ID: id, PolicyID: policyID, Status: "ready", CreatedAt: s.opts.Now().Add(-time.Duration(hours) * time.Hour)}
		require.NoError(t, os.MkdirAll(s.data(id), 0700))
		require.NoError(t, atomicJSON(s.record("snapshots", id), j))
		return id
	}
	a1, a2, b1, b2 := seed(a.ID, 1), seed(a.ID, 2), seed(b.ID, 1), seed(b.ID, 2)
	require.NoError(t, s.cleanup(context.Background()))
	for _, id := range []string{a1, a2, b1} {
		j, e := s.snapshot(id)
		require.NoError(t, e)
		require.Equal(t, "ready", j.Status)
		_, e = os.Stat(s.data(id))
		require.NoError(t, e)
	}
	j, e := s.snapshot(b2)
	require.NoError(t, e)
	require.Equal(t, "pruned", j.Status)
	_, e = os.Stat(s.data(b2))
	require.True(t, errors.Is(e, os.ErrNotExist))
}
