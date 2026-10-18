package snapshot_ops

import (
	"context"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestManualCaptureUsesSystemPolicyWithScheduleOff(t *testing.T) {
	s, backend, _ := fixtureService(t)
	ctx := rootContext()
	_, e := s.StartForPolicy(ctx, uuid.NewString(), "")
	require.Equal(t, "SNAPSHOT_POLICY_REQUIRED", dto.CodeOf(e))
	_, e = s.StartForPolicy(ctx, uuid.NewString(), uuid.NewString())
	require.Equal(t, "SNAPSHOT_POLICY_REQUIRED", dto.CodeOf(e))
	a, e := s.PutPolicy(ctx, "", "Daily", defaultPolicy())
	require.NoError(t, e)
	_, e = s.StartForPolicy(context.Background(), uuid.NewString(), a.ID)
	require.Equal(t, "SNAPSHOT_ROOT_REQUIRED", dto.CodeOf(e))
	require.NoError(t, s.Tick(ctx), "all schedules are disabled")
	_, e = s.StartForPolicy(ctx, uuid.NewString(), uuid.NewString())
	require.Equal(t, "SNAPSHOT_POLICY_CHANGED", dto.CodeOf(e))
	ids, e := s.ids("snapshots")
	require.NoError(t, e)
	require.Empty(t, ids, "rejected policy selections must not create a ledger or package")

	key := uuid.NewString()
	first, e := s.StartForPolicy(ctx, key, a.ID)
	require.NoError(t, e)
	require.Equal(t, a.ID, first.PolicyID)
	require.Equal(t, a.Name, first.PolicyName)
	require.Eventually(t, func() bool {
		row, err := s.Get(ctx, first.ID)
		return err == nil && row.Status == "ready"
	}, 10*time.Second, 10*time.Millisecond)
	waitIdle(t, s)
	_, e = s.StartForPolicy(ctx, "not-a-key", a.ID)
	require.Equal(t, "SNAPSHOT_IDEMPOTENCY_REQUIRED", dto.CodeOf(e))
	b, e := s.PutPolicy(ctx, "", "Before upgrade", defaultPolicy())
	require.NoError(t, e)
	_, e = s.SetDefaultPolicy(ctx, b.ID)
	require.NoError(t, e)
	_, e = s.StartForPolicy(ctx, uuid.NewString(), a.ID)
	require.Equal(t, "SNAPSHOT_POLICY_CHANGED", dto.CodeOf(e))
	replayed, e := s.StartForPolicy(ctx, key, a.ID)
	require.NoError(t, e, "a lost-response retry still returns its original task after a policy switch")
	require.Equal(t, first.ID, replayed.ID)
	_, e = s.StartForPolicy(ctx, key, b.ID)
	require.Equal(t, "SNAPSHOT_IDEMPOTENCY_CONFLICT", dto.CodeOf(e))
	require.EqualValues(t, 1, backend.captures.Load())
	second, e := s.StartForPolicy(ctx, uuid.NewString(), b.ID)
	require.NoError(t, e)
	require.Equal(t, b.ID, second.PolicyID)
	require.NotEqual(t, first.ID, second.ID)
	_, e = s.SetPolicyEnabled(ctx, b.ID, false)
	require.NoError(t, e)
	current, e := s.Overview(ctx)
	require.NoError(t, e)
	require.False(t, current.Policy.Enabled, "manual captures do not enable automatic scheduling")
	require.Nil(t, current.NextRunAt)
}
