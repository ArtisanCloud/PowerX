package agent_session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

type flakyAdmissionStore struct {
	*agent_run.RedisStore
	failCreate bool
}

type flakyPlanningQueue struct {
	agent_run.TaskEnqueuer
	fail bool
}

func (q *flakyPlanningQueue) Enqueue(ctx context.Context, message event_bus.TaskMessage) error {
	if q.fail {
		return errors.New("queue_unavailable")
	}
	return q.TaskEnqueuer.Enqueue(ctx, message)
}

func (s *flakyAdmissionStore) Create(ctx context.Context, run agent_run.Snapshot) (agent_run.Snapshot, error) {
	if s.failCreate {
		return agent_run.Snapshot{}, errors.New("redis_unavailable")
	}
	return s.RedisStore.Create(ctx, run)
}

func TestDurableAdmissionRepairsOnlyUnpublishedAnchor(t *testing.T) {
	svc, db, ctx, agent := fixture(t)
	tenant := reqctx.GetTenantUUID(ctx)
	require.NoError(t, db.Create(&capmodel.CapabilityRecord{CapabilityID: InvokeCapability, PluginID: "core", PluginVersion: "1", Title: "test", CapabilitiesHash: "hash", ProtocolHash: "hash", Status: "published"}).Error)
	require.NoError(t, db.Create(&capmodel.CapabilityRegistration{TenantUUID: tenant, CapabilityID: InvokeCapability, Status: "published", ContractRef: "test", Version: 1}).Error)
	require.NoError(t, db.Model(&setting.PluginInstanceConfig{}).Where("tenant_uuid = ? AND plugin_id = ?", tenant, "plugin.test").
		Update("value_json", datatypes.JSON([]byte(`{"client_id":"plugin.test","allowed_capabilities":["com.corex.agent.session.manage","com.corex.agent.invoke"]}`))).Error)
	session, err := svc.Create(ctx, agent, "")
	require.NoError(t, err)
	message, err := svc.Append(ctx, session.SessionUUID, "m", "user", "test")
	require.NoError(t, err)
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, err := agent_run.NewRedisStore(client)
	require.NoError(t, err)
	queue, err := event_bus.NewRedisStreamTaskDriver(client, time.Second)
	require.NoError(t, err)
	planningQueue := &flakyPlanningQueue{TaskEnqueuer: queue}
	flaky := &flakyAdmissionStore{RedisStore: store, failCreate: true}
	require.NoError(t, svc.ConfigureDurableAdmission(flaky, planningQueue, "dev", 30*time.Minute))
	_, err = svc.Invoke(ctx, session.SessionUUID, message.MessageUUID, "same-run")
	require.ErrorIs(t, err, ErrDependency)
	var anchor m.ServiceInvocation
	require.NoError(t, db.Where("session_uuid = ? AND idempotency_key = ?", session.SessionUUID, "same-run").First(&anchor).Error)
	require.Equal(t, 0, anchor.DeadlineAt.Nanosecond()%1000)
	require.Equal(t, "pending_create", anchor.AdmissionState)
	require.Equal(t, "accepted", anchor.Status)
	_, err = svc.Invoke(ctx, session.SessionUUID, message.MessageUUID, "other-run")
	require.ErrorIs(t, err, ErrConflict)
	flaky.failCreate = false
	planningQueue.fail = true
	_, err = svc.Invoke(ctx, session.SessionUUID, message.MessageUUID, "same-run")
	require.ErrorIs(t, err, ErrDependency)
	require.NoError(t, db.First(&anchor, "uuid = ?", anchor.UUID).Error)
	require.Equal(t, "admitted", anchor.AdmissionState)
	planningQueue.fail = false
	admitted, err := svc.Invoke(ctx, session.SessionUUID, message.MessageUUID, "same-run")
	require.NoError(t, err)
	require.Equal(t, anchor.UUID, admitted.InvocationUUID)
	require.Equal(t, "accepted", admitted.Status)
	require.NoError(t, db.First(&anchor, "uuid = ?", anchor.UUID).Error)
	require.Equal(t, "admitted", anchor.AdmissionState)
	repeat, err := svc.Invoke(ctx, session.SessionUUID, message.MessageUUID, "same-run")
	require.NoError(t, err)
	require.Equal(t, admitted.InvocationUUID, repeat.InvocationUUID)
	read, err := svc.GetInvocation(ctx, session.SessionUUID, anchor.UUID)
	require.NoError(t, err)
	require.Equal(t, "planning", read.Status)
	events, err := store.Events(ctx, tenant, "dev", anchor.UUID.String(), 0, 10)
	require.NoError(t, err)
	require.Len(t, events, 2)
	deliveries, err := queue.Dequeue(ctx, tenant+":dev", agent_run.AgentTaskSubscriber, "planning", "worker-a", 10, time.Millisecond)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	var count int64
	require.NoError(t, db.Model(&m.ServiceInvocation{}).Where("session_uuid = ?", session.SessionUUID).Count(&count).Error)
	require.EqualValues(t, 1, count)
	messages, total, err := svc.Messages(ctx, session.SessionUUID, 1, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, messages, 1)
	cancelled, err := svc.Cancel(ctx, session.SessionUUID, anchor.UUID)
	require.NoError(t, err)
	require.Equal(t, "cancelled", cancelled.Status)
	require.NoError(t, client.FlushAll(ctx).Err())
	_, err = svc.Invoke(ctx, session.SessionUUID, message.MessageUUID, "same-run")
	require.ErrorIs(t, err, ErrDependency)
	_, err = svc.GetInvocation(ctx, session.SessionUUID, anchor.UUID)
	require.ErrorIs(t, err, ErrDependency)
	_, err = store.Get(ctx, tenant, "dev", anchor.UUID.String())
	require.ErrorIs(t, err, agent_run.ErrNotFound)
}

func TestDurableAdmissionRejectsProcessLocalExecutor(t *testing.T) {
	svc := NewServiceWithExecutor(nil, &controlledExecutor{})
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	t.Cleanup(func() { _ = client.Close() })
	store, err := agent_run.NewRedisStore(client)
	require.NoError(t, err)
	queue, err := event_bus.NewRedisStreamTaskDriver(client, time.Second)
	require.NoError(t, err)
	require.ErrorIs(t, svc.ConfigureDurableAdmission(store, queue, "dev", 30*time.Minute), ErrInvalid)
	require.ErrorIs(t, svc.ConfigureDurableAdmission(store, queue, "bad env", 30*time.Minute), ErrInvalid)
}
