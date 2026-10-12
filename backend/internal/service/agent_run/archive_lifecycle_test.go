package agent_run

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestArchiveAdmissionGateUsesSharedHealthAndFailsClosed(t *testing.T) {
	ctx := context.Background()
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	first, _ := NewRedisStore(client)
	second, _ := NewRedisStore(client)
	now := time.Now().UTC()
	first.clock = func() time.Time { return now }
	second.clock = first.clock
	policy := ArchivePolicy{HotRetention: time.Hour, MaxPending: 2, MaxAge: time.Minute, HealthTTL: 3 * time.Second}
	require.ErrorIs(t, second.CheckArchiveAdmission(ctx, "dev", policy), ErrArchiveHealthStale)
	require.NoError(t, first.PublishArchiveHealth(ctx, "dev", ArchiveHealth{Pending: 1}, policy))
	require.NoError(t, second.CheckArchiveAdmission(ctx, "dev", policy))
	mismatch := policy
	mismatch.MaxPending++
	require.ErrorIs(t, second.CheckArchiveAdmission(ctx, "dev", mismatch), ErrArchiveHealthStale, "Core configuration disagreement must fail closed")
	require.NoError(t, first.PublishArchiveHealth(ctx, "dev", ArchiveHealth{Pending: 2}, policy))
	require.ErrorIs(t, second.CheckArchiveAdmission(ctx, "dev", policy), ErrArchiveBackpressure)
	old := now.Add(-time.Minute)
	require.NoError(t, first.PublishArchiveHealth(ctx, "dev", ArchiveHealth{Pending: 1, OldestAt: &old}, policy))
	require.ErrorIs(t, second.CheckArchiveAdmission(ctx, "dev", policy), ErrArchiveBackpressure)
	require.NoError(t, first.PublishArchiveHealth(ctx, "dev", ArchiveHealth{}, policy))
	require.NoError(t, second.CheckArchiveAdmission(ctx, "dev", policy))
	now = now.Add(4 * time.Second)
	require.ErrorIs(t, second.CheckArchiveAdmission(ctx, "dev", policy), ErrArchiveHealthStale)
	require.NoError(t, first.PublishArchiveHealth(ctx, "dev", ArchiveHealth{}, policy))
	mini.FastForward(4 * time.Second)
	require.ErrorIs(t, second.CheckArchiveAdmission(ctx, "dev", policy), ErrArchiveHealthStale)
}

func TestArchivedHotRetentionVerifiesAndExpiresOnlyOneRun(t *testing.T) {
	ctx := context.Background()
	mini := miniredis.RunT(t)
	now := time.Now().UTC()
	mini.SetTime(now)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, _ := NewRedisStore(client)
	store.clock = func() time.Time { return now }
	input := Snapshot{TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(), SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(), Status: "accepted", DeadlineAt: now.Add(3 * time.Hour)}
	run, err := store.Create(ctx, input)
	require.NoError(t, err)
	objects := &memoryReportObjects{items: map[string][]byte{}}
	require.Error(t, store.ExpireArchivedRun(ctx, run, objects, now.Add(time.Hour)))
	active := input
	active.RunID = uuid.NewString()
	active, err = store.Create(ctx, active)
	require.NoError(t, err)
	run, err = store.Transition(ctx, run, run.Version, "cancelled", "agent_run.cancelled")
	require.NoError(t, err)
	_, err = store.ArchiveRun(ctx, run, objects)
	require.NoError(t, err)
	run, err = store.Get(ctx, run.TenantUUID, run.Env, run.RunID)
	require.NoError(t, err)
	at := run.ArchivedAt.Add(time.Hour)
	require.NoError(t, store.ExpireArchivedRun(ctx, run, objects, at))
	state, events := runKeys(run)
	require.InDelta(t, time.Hour.Seconds(), mini.TTL(state).Seconds(), 1)
	now = now.Add(time.Minute)
	mini.FastForward(time.Minute)
	mini.SetTime(now)
	require.NoError(t, store.ExpireArchivedRun(ctx, run, objects, at))
	require.InDelta(t, (59 * time.Minute).Seconds(), mini.TTL(state).Seconds(), 1, "retry must not extend retention")
	objects.items[run.ArchiveKey] = []byte(`{}`)
	require.Error(t, store.ExpireArchivedRun(ctx, run, objects, at))
	mini.FastForward(time.Hour)
	now = now.Add(time.Hour)
	_, err = store.Get(ctx, run.TenantUUID, run.Env, run.RunID)
	require.ErrorIs(t, err, ErrNotFound)
	require.False(t, mini.Exists(events))
	_, err = store.Get(ctx, active.TenantUUID, active.Env, active.RunID)
	require.NoError(t, err, "other run must stay intact")
}
