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
