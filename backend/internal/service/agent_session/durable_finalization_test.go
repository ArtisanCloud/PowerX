package agent_session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/agent"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type finalOutputFunc func(context.Context, agent_run.Snapshot) (DurableOutput, error)

func (f finalOutputFunc) ReadFinalOutput(ctx context.Context, run agent_run.Snapshot) (DurableOutput, error) {
	return f(ctx, run)
}

func TestDurableFinalizationRetriesAtomicallyAndWritesOneMessage(t *testing.T) {
	svc, db, ctx, agent := fixture(t)
	session, err := svc.Create(ctx, agent, "review")
	require.NoError(t, err)
	message, err := svc.Append(ctx, session.SessionUUID, "input", "user", "review")
	require.NoError(t, err)
	client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, err := agent_run.NewRedisStore(client)
	require.NoError(t, err)
	queue, err := event_bus.NewRedisStreamTaskDriver(client, time.Second)
	require.NoError(t, err)
	require.NoError(t, svc.ConfigureDurableAdmission(store, queue, "dev", time.Hour))
	tenant := uuid.MustParse(reqctx.GetTenantUUID(ctx))
	anchor := m.ServiceInvocation{TenantUUID: tenant, SessionUUID: session.SessionUUID, MessageUUID: message.MessageUUID, AgentUUID: agent, PluginID: "plugin.test", ServiceActor: "client:plugin.test", Status: "accepted", RunEnv: "dev", AdmissionState: "admitted", IdempotencyKey: "review", RequestHash: "review", IdempotencyExpiresAt: time.Now().Add(time.Hour), DeadlineAt: time.Now().Add(time.Minute).UTC(), TraceUUID: uuid.New()}
	require.NoError(t, db.Create(&anchor).Error)
	identity := durableIdentity(repo.SessionOwner{TenantUUID: tenant}, &anchor)
	run, err := store.Create(ctx, identity)
	require.NoError(t, err)
	_, err = store.Transition(ctx, run, run.Version, "completed", "agent_run.completed")
	require.NoError(t, err)
	unavailable := true
	require.NoError(t, svc.ConfigureDurableOutput(finalOutputFunc(func(context.Context, agent_run.Snapshot) (DurableOutput, error) {
		if unavailable {
			return DurableOutput{}, errors.New("object_store_unavailable")
		}
		return DurableOutput{ResponseEnvelope: []byte(`{"schema":"test-report","presentation":{}}`)}, nil
	})))
	require.ErrorContains(t, svc.FinalizeRun(ctx, identity), "object_store_unavailable")
	require.NoError(t, db.First(&anchor, anchor.ID).Error)
	require.Nil(t, anchor.FinishedAt)
	unavailable = false
	// A transaction failure after inserting the message must roll the whole write back.
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_final_write BEFORE UPDATE ON agent_service_invocations BEGIN SELECT RAISE(ABORT, 'temporary write failure'); END`).Error)
	require.Error(t, svc.FinalizeRun(ctx, identity))
	var count int64
	require.NoError(t, db.Model(&m.ServiceMessage{}).Where("session_uuid = ? AND role = ?", session.SessionUUID, "assistant").Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Exec(`DROP TRIGGER reject_final_write`).Error)
	for i := 0; i < 3; i++ {
		require.NoError(t, svc.FinalizeRun(ctx, identity))
	}
	messages, total, err := svc.Messages(ctx, session.SessionUUID, 1, 10)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, messages, 2)
	require.Empty(t, messages[1].Content)
	require.JSONEq(t, `{"schema":"test-report","presentation":{}}`, string(messages[1].ResponseEnvelope))
	require.NoError(t, db.First(&anchor, anchor.ID).Error)
	require.Equal(t, "completed", anchor.Status)
	require.NotNil(t, anchor.FinishedAt)
	locators, err := svc.ListUnfinishedRuns(ctx, 0, 200)
	require.NoError(t, err)
	require.Empty(t, locators)
	// 归档阶段启用后，已写结果但未保存归档定位的 Run 仍可扫描恢复。
	objects := &sessionArchiveObjects{items: map[string][]byte{}}
	svc.EnableArchiveRecovery(objects)
	locators, err = svc.ListUnfinishedRuns(ctx, 0, 200)
	require.NoError(t, err)
	require.Len(t, locators, 1)
	_, err = store.ArchiveRun(ctx, identity, objects)
	require.NoError(t, err)
	archived, err := store.Get(ctx, identity.TenantUUID, identity.Env, identity.RunID)
	require.NoError(t, err)
	wrongMessage := archived
	wrongMessage.MessageID = uuid.NewString()
	require.ErrorIs(t, svc.RecordArchivedRun(ctx, wrongMessage), ErrConflict)
	for i := 0; i < 2; i++ {
		require.NoError(t, svc.RecordArchivedRun(ctx, archived))
	}
	changedKey := archived
	changedKey.ArchiveKey = "agent-runs/different-report.json"
	require.Error(t, svc.RecordArchivedRun(ctx, changedKey))
	locators, err = svc.ListUnfinishedRuns(ctx, 0, 200)
	require.NoError(t, err)
	require.Empty(t, locators)
	require.NoError(t, db.First(&anchor, anchor.ID).Error)
	require.Equal(t, archived.ArchiveKey, anchor.ArchiveKey)
	require.NotNil(t, anchor.ArchivedAt)
	// 独占 miniredis 清空后，正式 Query 和 SSE reader 从已验证归档恢复。
	require.NoError(t, client.FlushDB(ctx).Err())
	recovered, err := svc.GetInvocation(ctx, session.SessionUUID, anchor.UUID)
	require.NoError(t, err)
	require.Equal(t, "completed", recovered.Status)
	subscription, err := svc.OpenRunSubscription(ctx, session.SessionUUID, anchor.UUID)
	require.NoError(t, err)
	snapshot, events, err := subscription.Read(ctx, 0, 1000)
	require.NoError(t, err)
	require.Equal(t, "completed", snapshot.Status)
	require.Len(t, events, int(snapshot.EventSeq))
	state, err := subscription.SnapshotState(ctx)
	require.NoError(t, err)
	require.Equal(t, snapshot.RunID, state.RunID)
	_, _, err = subscription.Read(ctx, snapshot.EventSeq+1, 1000)
	require.ErrorIs(t, err, ErrEventCursorExpired)

	crossTenant := identity
	crossTenant.TenantUUID = uuid.NewString()
	require.ErrorIs(t, svc.FinalizeRun(ctx, crossTenant), ErrNotFound)
}

func TestDurableScannerRecoversPendingAdmissionsAndExpiresWithoutExecution(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(map[bool]string{false: "not_started", true: "deadline_elapsed"}[expired], func(t *testing.T) {
			svc, db, ctx, agent := fixture(t)
			session, err := svc.Create(ctx, agent, "pending recovery")
			require.NoError(t, err)
			message, err := svc.Append(ctx, session.SessionUUID, "input", "user", "review")
			require.NoError(t, err)
			client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
			t.Cleanup(func() { _ = client.Close() })
			store, err := agent_run.NewRedisStore(client)
			require.NoError(t, err)
			queue, err := event_bus.NewRedisStreamTaskDriver(client, time.Second)
			require.NoError(t, err)
			require.NoError(t, svc.ConfigureDurableAdmission(store, queue, "dev", time.Hour))
			require.NoError(t, svc.ConfigureDurableOutput(finalOutputFunc(func(context.Context, agent_run.Snapshot) (DurableOutput, error) {
				t.Fatal("expired run must not execute/read business output")
				return DurableOutput{}, nil
			})))
			tenant := uuid.MustParse(reqctx.GetTenantUUID(ctx))
			deadline := time.Now().Add(time.Hour).Truncate(time.Microsecond)
			if expired {
				deadline = time.Now().Add(-time.Minute).Truncate(time.Microsecond)
			}
			anchor := m.ServiceInvocation{TenantUUID: tenant, SessionUUID: session.SessionUUID, MessageUUID: message.MessageUUID, AgentUUID: agent, PluginID: "plugin.test", ServiceActor: "client:plugin.test", Status: "accepted", RunEnv: "dev", AdmissionState: "pending_create", IdempotencyKey: "pending", RequestHash: "pending", IdempotencyExpiresAt: time.Now().Add(time.Hour), DeadlineAt: deadline, TraceUUID: uuid.New()}
			require.NoError(t, db.Create(&anchor).Error)
			owner := repo.SessionOwner{TenantUUID: tenant, PluginID: anchor.PluginID, ServiceActor: anchor.ServiceActor}
			require.NoError(t, svc.repo.WithSession(ctx, owner, session.SessionUUID, func(tx *repo.ServiceSessionRepository, sess *m.ServiceSession) error {
				return tx.ExpireInvocations(ctx, sess, time.Now())
			}))
			require.NoError(t, db.First(&anchor, anchor.ID).Error)
			require.Nil(t, anchor.FinishedAt)
			rows, err := svc.ListUnfinishedRuns(ctx, 0, 200)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			require.NoError(t, db.First(&anchor, anchor.ID).Error)
			require.Equal(t, "admitted", anchor.AdmissionState)
			run, err := store.Get(ctx, tenant.String(), "dev", anchor.UUID.String())
			require.NoError(t, err)
			if expired {
				require.Equal(t, "failed", run.Status)
				events, err := store.Events(ctx, run.TenantUUID, run.Env, run.RunID, 0, 10)
				require.NoError(t, err)
				require.Len(t, events, 1)
				require.Equal(t, "run.deadline", events[0].ReasonCode)
				require.NoError(t, svc.FinalizeRun(ctx, run))
				rows, err = svc.ListUnfinishedRuns(ctx, 0, 200)
				require.NoError(t, err)
				require.Empty(t, rows)
			} else {
				require.Equal(t, "accepted", run.Status)
				again, err := svc.ListUnfinishedRuns(ctx, 0, 200)
				require.NoError(t, err)
				require.Len(t, again, 1)
				current, err := store.Get(ctx, run.TenantUUID, run.Env, run.RunID)
				require.NoError(t, err)
				require.Equal(t, run.Version, current.Version)
			}
		})
	}
}

type sessionArchiveObjects struct{ items map[string][]byte }

func (s *sessionArchiveObjects) Put(_ context.Context, key string, body []byte) error {
	s.items[key] = append([]byte(nil), body...)
	return nil
}
func (s *sessionArchiveObjects) Get(_ context.Context, key string) ([]byte, error) {
	body, ok := s.items[key]
	if !ok {
		return nil, agent_run.ErrNotFound
	}
	return append([]byte(nil), body...), nil
}
