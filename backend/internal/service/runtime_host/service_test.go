package runtime_host

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	fixture "github.com/ArtisanCloud/PowerX/internal/testutil/runtime_host"
	capm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/runtime_host"
	tenantmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func kind(t *testing.T, err error, want string) {
	t.Helper()
	var e *Error
	require.ErrorAs(t, err, &e)
	require.Equal(t, want, e.Kind)
}
func TestTaskPersistenceIsolationAndRevocation(t *testing.T) {
	db := fixture.Database(t)
	tenant := uuid.NewString()
	ctx, credential := fixture.Actor(t, db, tenant, "plugin.one")
	other, _ := fixture.Actor(t, db, tenant, "plugin.two")
	cross, _ := fixture.Actor(t, db, uuid.NewString(), "plugin.one")
	s := NewService(db, nil)
	in := CreateTaskInput{Type: "export", IdempotencyKey: "first", Payload: json.RawMessage(`{"x":1,"y":2}`)}
	task, e := s.CreateTask(ctx, in)
	require.NoError(t, e)
	require.NotEqual(t, uuid.Nil, task.TaskUUID)
	require.EqualValues(t, 1, task.Revision)
	require.Equal(t, "queued", task.State)
	in.Payload = json.RawMessage(`{ "y":2, "x":1 }`)
	same, e := s.CreateTask(ctx, in)
	require.NoError(t, e)
	require.Equal(t, task.TaskUUID, same.TaskUUID)
	in.Payload = json.RawMessage(`{"x":2}`)
	_, e = s.CreateTask(ctx, in)
	kind(t, e, "conflict")
	in.Type = "different"
	_, e = s.CreateTask(ctx, in)
	kind(t, e, "conflict")
	for _, c := range []context.Context{other, cross} {
		_, e = s.GetTask(c, task.TaskUUID.String())
		kind(t, e, "not_found")
		_, e = s.UpdateTask(c, task.TaskUUID.String(), UpdateTaskInput{ExpectedRevision: 1, State: "running", Progress: 1})
		kind(t, e, "not_found")
	}
	id := task.TaskUUID.String()
	_, e = s.UpdateTask(ctx, id, UpdateTaskInput{ExpectedRevision: 1, State: "succeeded", Progress: 100})
	kind(t, e, "conflict")
	running, e := s.UpdateTask(ctx, id, UpdateTaskInput{ExpectedRevision: 1, State: "running", Progress: 50, MessageKey: "tasks.running"})
	require.NoError(t, e)
	require.EqualValues(t, 2, running.Revision)
	_, e = s.UpdateTask(ctx, id, UpdateTaskInput{ExpectedRevision: 1, State: "failed", Progress: 50})
	kind(t, e, "conflict")
	_, e = s.UpdateTask(ctx, id, UpdateTaskInput{ExpectedRevision: 2, State: "running", Progress: 49})
	kind(t, e, "conflict")
	_, e = s.UpdateTask(ctx, id, UpdateTaskInput{ExpectedRevision: 2, State: "queued", Progress: 50})
	kind(t, e, "conflict")
	done, e := s.UpdateTask(ctx, id, UpdateTaskInput{ExpectedRevision: 2, State: "succeeded", Progress: 100, Result: json.RawMessage(`{"count":2}`)})
	require.NoError(t, e)
	require.NotNil(t, done.CompletedAt)
	_, e = s.UpdateTask(ctx, id, UpdateTaskInput{ExpectedRevision: 3, State: "failed", Progress: 100})
	kind(t, e, "conflict")
	persisted, e := NewService(db, nil).GetTask(ctx, id)
	require.NoError(t, e)
	require.Equal(t, done.TaskUUID, persisted.TaskUUID)
	require.Equal(t, done.Revision, persisted.Revision)
	require.Equal(t, done.State, persisted.State)
	require.JSONEq(t, string(done.Result), string(persisted.Result))
	require.WithinDuration(t, done.UpdatedAt, persisted.UpdatedAt, time.Microsecond)
	require.WithinDuration(t, *done.CompletedAt, *persisted.CompletedAt, time.Microsecond)
	fixture.Grant(t, db, credential)
	_, e = s.GetTask(ctx, id)
	kind(t, e, "forbidden")
	_, e = s.CreateTask(ctx, in)
	kind(t, e, "forbidden")
	_, e = s.UpdateTask(ctx, id, UpdateTaskInput{})
	kind(t, e, "forbidden")
	fixture.Grant(t, db, credential, fixture.Capabilities...)
	require.NoError(t, db.Create(&capm.CapabilityRegistration{CapabilityID: TaskRead, TenantUUID: tenant, ContractRef: "v2", Status: "disabled", Version: 2, RoutingPolicyID: uuid.New()}).Error)
	_, e = s.GetTask(ctx, id)
	kind(t, e, "forbidden")
	var operations int64
	require.NoError(t, db.Model(&m.Operation{}).Where("task_uuid = ?", id).Count(&operations).Error)
	require.EqualValues(t, 3, operations)
}
func TestTaskConcurrentCASAndIdempotency(t *testing.T) {
	db := fixture.Database(t)
	ctx, _ := fixture.Actor(t, db, uuid.NewString(), "plugin.concurrent")
	s := NewService(db, nil)
	const count = 8
	var wg sync.WaitGroup
	tasks := make(chan *Task, count)
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			task, e := s.CreateTask(ctx, CreateTaskInput{Type: "export", IdempotencyKey: "parallel", Payload: json.RawMessage(`{}`)})
			tasks <- task
			errs <- e
		}()
	}
	wg.Wait()
	close(tasks)
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	id := ""
	for task := range tasks {
		if id == "" {
			id = task.TaskUUID.String()
		}
		require.Equal(t, id, task.TaskUUID.String())
	}
	errs = make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := s.UpdateTask(ctx, id, UpdateTaskInput{ExpectedRevision: 1, State: "running", Progress: 10})
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	success := 0
	for e := range errs {
		if e == nil {
			success++
		} else {
			kind(t, e, "conflict")
		}
	}
	require.Equal(t, 1, success)
	task, e := s.GetTask(ctx, id)
	require.NoError(t, e)
	require.EqualValues(t, 2, task.Revision)
}
func TestCacheSemanticsAndLiveGrant(t *testing.T) {
	db := fixture.Database(t)
	ctx, credential := fixture.Actor(t, db, uuid.NewString(), "plugin.cache")
	other, _ := fixture.Actor(t, db, reqctx.GetTenantUUID(ctx), "plugin.other")
	cross, _ := fixture.Actor(t, db, uuid.NewString(), "plugin.cache")
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { client.Close() })
	s := NewService(db, client)
	out, e := s.GetCache(ctx, "ns", "empty")
	require.NoError(t, e)
	require.False(t, out.Found)
	require.NoError(t, s.SetCache(ctx, "ns", "empty", "", 1000))
	out, e = s.GetCache(ctx, "ns", "empty")
	require.NoError(t, e)
	require.True(t, out.Found)
	require.Empty(t, out.ValueBase64)
	require.NotNil(t, out.ExpiresAt)
	for _, c := range []context.Context{other, cross} {
		out, e = s.GetCache(c, "ns", "empty")
		require.NoError(t, e)
		require.False(t, out.Found)
		require.NoError(t, s.DeleteCache(c, "ns", "empty"))
	}
	out, e = s.GetCache(ctx, "ns", "empty")
	require.NoError(t, e)
	require.True(t, out.Found)
	server.FastForward(time.Second)
	out, e = s.GetCache(ctx, "ns", "empty")
	require.NoError(t, e)
	require.False(t, out.Found)
	require.Nil(t, out.ExpiresAt)
	require.NoError(t, s.DeleteCache(ctx, "ns", "absent"))
	require.NoError(t, s.SetCache(ctx, "ns", "bytes", "AP8K", 1000))
	out, e = s.GetCache(ctx, "ns", "bytes")
	require.NoError(t, e)
	require.Equal(t, "AP8K", out.ValueBase64)
	fixture.Grant(t, db, credential)
	_, e = s.GetCache(ctx, "ns", "bytes")
	kind(t, e, "forbidden")
	kind(t, s.SetCache(ctx, "ns", "bytes", "", 1000), "forbidden")
	kind(t, s.DeleteCache(ctx, "ns", "bytes"), "forbidden")
	fixture.Grant(t, db, credential, fixture.Capabilities...)
	server.Close()
	_, e = s.GetCache(ctx, "ns", "bytes")
	kind(t, e, "upstream_dependency")
	kind(t, s.DeleteCache(ctx, "ns", "bytes"), "upstream_dependency")
}
func TestInvalidInputAndDependencyFailure(t *testing.T) {
	db := fixture.Database(t)
	ctx, _ := fixture.Actor(t, db, uuid.NewString(), "plugin.validation")
	s := NewService(db, nil)
	_, e := s.CreateTask(context.Background(), CreateTaskInput{})
	kind(t, e, "unauthorized")
	forged := *reqctx.GetClaims(ctx)
	forged.Subject = "client:someone_else"
	_, e = s.CreateTask(reqctx.WithClaims(ctx, &forged), CreateTaskInput{})
	kind(t, e, "unauthorized")
	_, e = s.GetTask(reqctx.WithTenantUUID(ctx, uuid.NewString()), uuid.NewString())
	kind(t, e, "unauthorized")
	for _, raw := range []string{``, `{"x":1,"x":2}`, `{"x":{"a":1,"a":2}}`, `{} {}`, `[`, `NaN`} {
		_, e = s.CreateTask(ctx, CreateTaskInput{Type: "t", IdempotencyKey: uuid.NewString(), Payload: json.RawMessage(raw)})
		kind(t, e, "invalid_argument")
	}
	for _, ttl := range []int64{0, -1, MaxTTLMillis + 1} {
		kind(t, s.SetCache(ctx, "n", "k", "", ttl), "invalid_argument")
	}
	kind(t, s.SetCache(ctx, "n", "k", "AB==", 1), "invalid_argument")
	kind(t, s.SetCache(ctx, "n", "k", "", 1), "upstream_dependency")
	_, e = s.GetTask(ctx, "task-123")
	kind(t, e, "invalid_argument")
	for _, state := range []string{"failed", "cancelled"} {
		task, e := s.CreateTask(ctx, CreateTaskInput{Type: "t", IdempotencyKey: state, Payload: json.RawMessage(`null`)})
		require.NoError(t, e)
		done, e := s.UpdateTask(ctx, task.TaskUUID.String(), UpdateTaskInput{ExpectedRevision: 1, State: state})
		require.NoError(t, e)
		require.NotNil(t, done.CompletedAt)
	}
	row := &m.Task{Revision: math.MaxInt64, State: "running"}
	require.Error(t, validateTransition(row, UpdateTaskInput{ExpectedRevision: math.MaxInt64, State: "running"}))
	unchanged, e := s.CreateTask(ctx, CreateTaskInput{Type: "t", IdempotencyKey: "rollback-update", Payload: json.RawMessage(`{}`)})
	require.NoError(t, e)
	require.NoError(t, db.Migrator().DropTable(&m.Operation{}))
	_, e = s.UpdateTask(ctx, unchanged.TaskUUID.String(), UpdateTaskInput{ExpectedRevision: 1, State: "running", Progress: 10})
	kind(t, e, "upstream_dependency")
	after, e := s.GetTask(ctx, unchanged.TaskUUID.String())
	require.NoError(t, e)
	require.EqualValues(t, 1, after.Revision)
	require.Equal(t, "queued", after.State)
	_, e = s.CreateTask(ctx, CreateTaskInput{Type: "rollback", IdempotencyKey: "rollback", Payload: json.RawMessage(`{}`)})
	kind(t, e, "upstream_dependency")
	var total int64
	require.NoError(t, db.Model(&m.Task{}).Where("idempotency_key = ?", "rollback").Count(&total).Error)
	require.Zero(t, total)
}

func TestHostAuthorizationAndLimits(t *testing.T) {
	db := fixture.Database(t)
	tenant := uuid.NewString()
	ctx, credential := fixture.Actor(t, db, tenant, "plugin.limits")
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer client.Close()
	s := NewService(db, client)
	kind(t, s.SetCache(ctx, strings.Repeat("n", 129), "k", "", 1), "invalid_argument")
	kind(t, s.SetCache(ctx, "n", strings.Repeat("k", 513), "", 1), "invalid_argument")
	kind(t, s.SetCache(ctx, "n", "k", strings.Repeat("A", 1398108), 1), "invalid_argument")
	_, e := s.CreateTask(ctx, CreateTaskInput{Type: "t", IdempotencyKey: "huge", Payload: json.RawMessage(`"` + strings.Repeat("x", MaxJSONBytes) + `"`)})
	kind(t, e, "invalid_argument")
	_, e = s.CreateTask(ctx, CreateTaskInput{Type: "t", IdempotencyKey: "nul", Payload: json.RawMessage(`{"x":"\u0000"}`)})
	kind(t, e, "invalid_argument")
	require.NoError(t, db.Model(&tenantmodel.Tenant{}).Where("uuid = ?", tenant).Update("status", 0).Error)
	_, e = s.GetCache(ctx, "n", "k")
	kind(t, e, "forbidden")
	require.NoError(t, db.Model(&tenantmodel.Tenant{}).Where("uuid = ?", tenant).Update("status", 1).Error)
	require.NoError(t, db.Model(&capm.CapabilityRecord{}).Where("capability_id = ?", CacheRead).Update("status", "draft").Error)
	_, e = s.GetCache(ctx, "n", "k")
	kind(t, e, "forbidden")
	require.NoError(t, db.Model(&capm.CapabilityRecord{}).Where("capability_id = ?", CacheRead).Update("status", "published").Error)
	require.NoError(t, db.Model(credential).Update("enabled", false).Error)
	_, e = s.GetCache(ctx, "n", "k")
	kind(t, e, "forbidden")
}
