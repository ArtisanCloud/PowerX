package storagegov

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	mevent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 仅连接 Agent 自有临时实例；Redis 使用内存仿真，不连接日常服务。
func ownedControlPostgres(t *testing.T) (*gorm.DB, *taskqueue.StreamDriver) {
	t.Helper()
	dsn := os.Getenv("POWERX_STORAGE_CONTROL_TEST_DSN")
	if dsn == "" {
		t.Skip("requires owned temporary PostgreSQL")
	}
	cfg, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(cfg.Host, "/tmp/powerx-control-owned-"), "refuse daily PostgreSQL")
	require.Equal(t, "storage_control_test", cfg.User)
	require.Equal(t, "postgres", cfg.Database)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(16)
	old := coremodel.PowerXSchema
	schema := "control_owned_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
	coremodel.PowerXSchema = schema
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
		coremodel.PowerXSchema = old
		require.NoError(t, pool.Close())
	})
	// 调用正式集中迁移挂载，重复执行验证幂等；只在自有新 schema 中运行。
	require.NoError(t, database.MigrateStorageGovernanceModels(db))
	require.NoError(t, database.MigrateStorageGovernanceModels(db))
	require.NoError(t, db.AutoMigrate(&mevent.TaskHistory{}))
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	base, err := event_bus.NewRedisStreamTaskDriver(client, 10*time.Second, "owned-control-contract")
	require.NoError(t, err)
	return db, taskqueue.NewStreamDriverWithRetryDelay(db, base, time.Millisecond)
}

func controlNative(t *testing.T, db *gorm.DB, queue *taskqueue.StreamDriver, opts ControlOptions, apply func(context.Context, ControlPublication) error) *ControlService {
	t.Helper()
	opts.Scope = ControlScope{HostID: "owned-host", FilesystemID: "owned-fs", InstanceID: "owned-install"}
	opts.Queue = queue
	s, err := NewControlService(db, opts)
	require.NoError(t, err)
	require.NoError(t, s.Bind(ControlLifecycle, controlBinding(apply)))
	return s
}

func controlCounts(t *testing.T, db *gorm.DB, expected int64) {
	t.Helper()
	for _, model := range []any{&m.ControlRevision{}, &mevent.TaskHistory{}} {
		var count int64
		require.NoError(t, db.Model(model).Count(&count).Error)
		require.Equal(t, expected, count)
	}
}

func TestControlNativePostgresCASIdempotencyAndRollback(t *testing.T) {
	db, queue := ownedControlPostgres(t)
	s := controlNative(t, db, queue, ControlOptions{}, func(context.Context, ControlPublication) error { return nil })
	ctx := controlRoot()
	_, err := s.Save(ctx, controlInput(0, 7))
	require.NoError(t, err)
	start := make(chan struct{})
	results := make(chan error, 12)
	var workers sync.WaitGroup
	for i := 0; i < 12; i++ {
		workers.Add(1)
		go func(days int) {
			defer workers.Done()
			<-start
			_, e := s.Save(ctx, controlInput(1, days))
			results <- e
		}(i + 14)
	}
	close(start)
	workers.Wait()
	close(results)
	winners := 0
	for e := range results {
		if e == nil {
			winners++
		} else {
			require.Equal(t, "STORAGE_CONTROL_REVISION_CONFLICT", dto.CodeOf(e))
		}
	}
	require.Equal(t, 1, winners)
	controlCounts(t, db, 2)
	in := controlInput(2, 30)
	results = make(chan error, 8)
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			out, e := s.Save(ctx, in)
			if e == nil && out.OperationRevision != 3 {
				e = errors.New("idempotency returned another receipt")
			}
			results <- e
		}()
	}
	workers.Wait()
	close(results)
	for e := range results {
		require.NoError(t, e)
	}
	controlCounts(t, db, 3)
	// 注入正式 TaskBus 登记失败，配置、审计、请求回执应全回滚。
	require.NoError(t, db.Migrator().DropTable(&mevent.TaskHistory{}))
	_, err = s.Save(ctx, controlInput(3, 60))
	require.Equal(t, "STORAGE_CONTROL_SAVE_UNAVAILABLE", dto.CodeOf(err))
	out, err := s.Read(ctx, ControlLifecycle)
	require.NoError(t, err)
	require.EqualValues(t, 3, out.DesiredRevision)
	var audits int64
	require.NoError(t, db.Model(&m.ControlRevision{}).Count(&audits).Error)
	require.EqualValues(t, 3, audits)
}

func TestControlNativePostgresTaskBusRecoverySupersessionAndConfirmation(t *testing.T) {
	db, queue := ownedControlPostgres(t)
	ctx := controlRoot()
	var applied []int64
	s := controlNative(t, db, queue, ControlOptions{Dispatch: func(context.Context, event_bus.TaskMessage) error { return errors.New("owned lost transport") }}, func(_ context.Context, p ControlPublication) error {
		applied = append(applied, p.Revision)
		return nil
	})
	for _, revision := range []int64{0, 1} {
		out, err := s.Save(ctx, controlInput(revision, 14))
		require.NoError(t, err)
		require.Equal(t, "pending", out.State)
		require.Zero(t, out.AppliedRevision)
	}
	require.NoError(t, queue.Recover(ctx, controlQueueTenant, s.subscriber()))
	items, err := queue.Dequeue(ctx, controlQueueTenant, s.subscriber(), "owned-test", "owned-worker", 10, time.Millisecond)
	require.NoError(t, err)
	require.Len(t, items, 2)
	for _, item := range items {
		require.NoError(t, s.ApplyDelivery(ctx, item))
	}
	require.Equal(t, []int64{2}, applied, "old desired revision must never publish")
	out, err := s.Read(ctx, ControlLifecycle)
	require.NoError(t, err)
	require.EqualValues(t, 2, out.AppliedRevision)
	require.Equal(t, "applied", out.State)
	for _, item := range items {
		status, e := queue.Status(ctx, controlQueueTenant, s.subscriber(), item.Message.Metadata["taskbus_record_id"])
		require.NoError(t, e)
		require.Equal(t, mevent.TaskHistoryStatusCompleted, status.Status)
	}
	require.NoError(t, queue.Recover(ctx, controlQueueTenant, s.subscriber()))
	items, err = queue.Dequeue(ctx, controlQueueTenant, s.subscriber(), "owned-test", "owned-worker", 10, time.Millisecond)
	require.NoError(t, err)
	require.Empty(t, items)
}

func TestControlNativePostgresFailedPublicationDoesNotClaimApplied(t *testing.T) {
	db, queue := ownedControlPostgres(t)
	ctx := controlRoot()
	fail := true
	s := controlNative(t, db, queue, ControlOptions{}, func(context.Context, ControlPublication) error {
		if fail {
			return errors.New("owned full filesystem")
		}
		return nil
	})
	_, err := s.Save(ctx, controlInput(0, 7))
	require.NoError(t, err)
	items, err := queue.Dequeue(ctx, controlQueueTenant, s.subscriber(), "owned-test", "owned-worker", 1, time.Millisecond)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Error(t, s.ApplyDelivery(ctx, items[0]))
	out, err := s.Read(ctx, ControlLifecycle)
	require.NoError(t, err)
	require.Equal(t, "failed", out.State)
	require.Zero(t, out.AppliedRevision)
	status, err := queue.Status(ctx, controlQueueTenant, s.subscriber(), items[0].Message.ID)
	require.NoError(t, err)
	require.Equal(t, mevent.TaskHistoryStatusRunning, status.Status, "failed business transaction cannot complete task")
	fail = false
	require.NoError(t, s.ApplyDelivery(ctx, items[0]))
	out, err = s.Read(ctx, ControlLifecycle)
	require.NoError(t, err)
	require.Equal(t, "applied", out.State)
	require.EqualValues(t, 1, out.AppliedRevision)
}

func TestControlNativePostgresWorkerRecoversCommittedIntentAndStops(t *testing.T) {
	db, queue := ownedControlPostgres(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	changes := make(chan int64, 4)
	problems := make(chan error, 4)
	var s *ControlService
	s = controlNative(t, db, queue, ControlOptions{
		Dispatch: func(context.Context, event_bus.TaskMessage) error { return errors.New("owned lost enqueue") },
		Changed: func() {
			state, err := s.Read(controlRoot(), ControlLifecycle)
			if err != nil {
				problems <- err
				return
			}
			changes <- state.AppliedRevision
		},
	}, func(context.Context, ControlPublication) error { return nil })
	_, err := s.Save(controlRoot(), controlInput(0, 7))
	require.NoError(t, err)
	require.EqualValues(t, 0, <-changes)
	s.Start(ctx, func(e error) {
		select {
		case problems <- e:
		default:
		}
	})
	t.Cleanup(func() {
		cancel()
		select {
		case <-s.done:
		case <-time.After(10 * time.Second):
			t.Error("owned TaskBus worker did not stop")
		}
	})
	select {
	case revision := <-changes:
		require.EqualValues(t, 1, revision)
	case e := <-problems:
		require.NoError(t, e)
	case <-time.After(10 * time.Second):
		t.Fatal("no confirmed publication event")
	}
	require.Error(t, s.Bind(ControlBudget, controlBinding(func(context.Context, ControlPublication) error { return nil })), "runtime cannot replace a sealed publisher")
}
