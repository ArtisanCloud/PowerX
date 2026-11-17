package storagebudget

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestReclamationOnlyBypassesSoftLimitAndPreservesOtherReservations(t *testing.T) {
	c := fixture(t, "owned", "", nil)
	ctx := context.Background()
	require.NoError(t, c.locked(ctx, true, func(s *ledger) error { s.Policy.MaxUsedBytes = 1; return nil }))
	_, e := c.Reserve(ctx, "ordinary", 1<<20, time.Minute)
	require.ErrorIs(t, e, ErrInsufficient)
	_, e = c.ReserveReclamation(ctx, "ordinary", 1, time.Minute)
	require.ErrorIs(t, e, ErrUnavailable)
	status, e := c.Status(ctx)
	require.NoError(t, e)
	_, e = c.ReserveReclamation(ctx, "database-compaction", status.AvailableBytes, time.Minute)
	require.ErrorIs(t, e, ErrInsufficient)
	l, e := c.ReserveReclamation(ctx, "database-compaction", 1<<20, time.Minute)
	require.NoError(t, e)
	_, e = c.ReserveReclamation(ctx, "database-vacuum", 1, time.Minute)
	require.ErrorIs(t, e, ErrBusy)
	require.ErrorIs(t, c.ReleaseReclamation(ctx, l.ID(), "database-vacuum"), ErrUnavailable)
	require.NoError(t, c.ReleaseReclamation(ctx, l.ID(), "database-compaction"))
	require.NoError(t, l.Release(ctx))
	_, e = c.Reserve(ctx, "ordinary", 1, time.Minute)
	require.ErrorIs(t, e, ErrInsufficient)
}

func TestVacuumUsesCriticalFloorWithoutChangingSharedPolicy(t *testing.T) {
	c := fixture(t, "owned", "", nil)
	ctx := context.Background()
	state, e := c.Status(ctx)
	require.NoError(t, e)
	policy, e := c.Policy(ctx)
	require.NoError(t, e)
	policy.MinFreeBytes = state.AvailableBytes + (1 << 30)
	policy.CriticalFreeBytes = state.AvailableBytes / 2
	policy.MaxUsedBytes = 1
	_, e = c.SavePolicy(ctx, policy.Revision, policy)
	require.NoError(t, e)
	_, e = c.Reserve(ctx, "backup", 1<<20, time.Minute)
	require.ErrorIs(t, e, ErrInsufficient)
	_, e = c.ReserveReclamation(ctx, "database-compaction", 1<<20, time.Minute)
	require.ErrorIs(t, e, ErrInsufficient)
	l, e := c.ReserveReclamation(ctx, "database-vacuum", 1<<20, time.Minute)
	require.NoError(t, e)
	_, e = c.ReserveReclamation(ctx, "database-vacuum", 1, time.Minute)
	require.ErrorIs(t, e, ErrBusy)
	require.NoError(t, l.Release(ctx))
	got, e := c.Policy(ctx)
	require.NoError(t, e)
	require.Equal(t, policy.MinFreeBytes, got.MinFreeBytes)
	require.Equal(t, policy.CriticalFreeBytes, got.CriticalFreeBytes)
	state, e = c.Status(ctx)
	require.NoError(t, e)
	_, e = c.ReserveReclamation(ctx, "database-vacuum", state.AvailableBytes-state.CriticalFreeBytes+1, time.Minute)
	require.ErrorIs(t, e, ErrInsufficient)
}
