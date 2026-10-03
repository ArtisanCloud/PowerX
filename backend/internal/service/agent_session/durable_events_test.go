package agent_session

import (
	"context"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestDurableRunEventsReplayAndOwnerBoundary(t *testing.T) {
	svc, db, ctx, agent := fixture(t)
	session, err := svc.Create(ctx, agent, "")
	require.NoError(t, err)
	message, err := svc.Append(ctx, session.SessionUUID, "m", "user", "test")
	require.NoError(t, err)
	tenant := uuid.MustParse(reqctx.GetTenantUUID(ctx))
	anchor := &m.ServiceInvocation{TenantUUID: tenant, SessionUUID: session.SessionUUID, AgentUUID: agent,
		MessageUUID: message.MessageUUID, PluginID: "plugin.test", ServiceActor: "client:plugin.test",
		Status: "running", IdempotencyKey: "run", RequestHash: message.MessageUUID.String(),
		IdempotencyExpiresAt: time.Now().Add(time.Hour), DeadlineAt: time.Now().Add(time.Hour), TraceUUID: uuid.New()}
	require.NoError(t, db.Create(anchor).Error)
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, err := agent_run.NewRedisStore(client)
	require.NoError(t, err)
	initial := agent_run.Snapshot{TenantUUID: tenant.String(), Env: "dev", RunID: anchor.UUID.String(),
		SessionID: session.SessionUUID.String(), MessageID: message.MessageUUID.String(), TraceID: anchor.TraceUUID.String(),
		Status: "accepted", DeadlineAt: anchor.DeadlineAt}
	created, err := store.Create(ctx, initial)
	require.NoError(t, err)
	_, err = store.Transition(ctx, initial, created.Version, "planning", "agent_run.plan_created")
	require.NoError(t, err)
	require.NoError(t, svc.ConfigureRunEvents(store, "dev"))
	run, events, err := svc.RunEvents(ctx, session.SessionUUID, anchor.UUID, 0, 100)
	require.NoError(t, err)
	require.EqualValues(t, 2, run.EventSeq)
	require.Len(t, events, 2)
	_, events, err = svc.RunEvents(ctx, session.SessionUUID, anchor.UUID, 1, 100)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.EqualValues(t, 2, events[0].Seq)
	_, _, err = svc.RunEvents(ctx, session.SessionUUID, anchor.UUID, 3, 100)
	require.ErrorIs(t, err, ErrEventCursorExpired)
	otherClaims := *reqctx.GetClaims(ctx)
	otherClaims.PluginID, otherClaims.Subject = "plugin.other", "client:plugin.other"
	other := reqctx.WithClaims(ctx, &otherClaims)
	_, _, err = svc.RunEvents(other, session.SessionUUID, anchor.UUID, 0, 100)
	require.ErrorIs(t, err, ErrNotFound)
	_, _, err = svc.RunEvents(context.Background(), session.SessionUUID, anchor.UUID, 0, 100)
	require.ErrorIs(t, err, ErrUnauthorized)
	eventsKey := agentRunEventKey(initial)
	require.NoError(t, client.XDel(ctx, eventsKey, "1-0").Err())
	run, events, err = svc.RunEvents(ctx, session.SessionUUID, anchor.UUID, 0, 100)
	require.ErrorIs(t, err, ErrEventCursorExpired)
	require.EqualValues(t, 2, run.EventSeq)
	require.Empty(t, events)
	subscription, err := svc.OpenRunSubscription(ctx, session.SessionUUID, anchor.UUID)
	require.NoError(t, err)
	state, err := subscription.SnapshotState(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 2, state.EventSeq)
	require.Empty(t, state.Tasks)
	redisServer.Close()
	_, _, err = svc.RunEvents(ctx, session.SessionUUID, anchor.UUID, 1, 100)
	require.ErrorIs(t, err, ErrDependency)
}

func agentRunEventKey(run agent_run.Snapshot) string {
	return "agent:run:{" + run.TenantUUID + ":" + run.Env + ":" + run.RunID + "}:events"
}
