package taskqueue

import (
	"context"
	"errors"
	"testing"
	"time"

	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func fixture(t *testing.T) (*StreamDriver, *gorm.DB, *miniredis.Miniredis) {
	t.Helper()
	old := core.PowerXSchema
	core.PowerXSchema = "main"
	t.Cleanup(func() { core.PowerXSchema = old })
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/tasks.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = pool.Close() })
	stmt := &gorm.Statement{DB: db}
	require.NoError(t, stmt.Parse(&m.TaskHistory{}))
	for _, field := range stmt.Schema.Fields {
		if field.DataType == "timestamp with time zone" {
			field.DataType = "datetime"
			field.TagSettings["TYPE"] = "datetime"
		}
	}
	require.NoError(t, db.AutoMigrate(&m.TaskHistory{}))
	require.NoError(t, db.Exec("CREATE TABLE fixture_results (value TEXT NOT NULL)").Error)
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	base, err := event_bus.NewRedisStreamTaskDriver(client, time.Second)
	require.NoError(t, err)
	return NewStreamDriverWithRetryDelay(db, base, time.Millisecond), db, mini
}
func message(id string) event_bus.TaskMessage {
	return event_bus.TaskMessage{ID: id, TenantKey: "core-worker", SubscriberID: "fixture", Topic: "fixture.work", Payload: []byte(`{"business_uuid":"opaque"}`), Metadata: map[string]string{recordTenantKey: "tenant-a", "kind": "fixture", "max_attempts": "3"}}
}
func dequeue(t *testing.T, d *StreamDriver) []event_bus.StreamTaskDelivery {
	t.Helper()
	out, err := d.Dequeue(context.Background(), "core-worker", "fixture", "workers", "worker", 10, time.Millisecond)
	require.NoError(t, err)
	return out
}

func TestSharedHistoryTransactionalResultAndRecoveryDedup(t *testing.T) {
	d, db, _ := fixture(t)
	ctx := context.Background()
	msg := message("work-1")
	rollback := errors.New("rollback")
	require.ErrorIs(t, db.Transaction(func(tx *gorm.DB) error { require.NoError(t, d.Register(ctx, tx, msg)); return rollback }), rollback)
	var count int64
	require.NoError(t, db.Model(&m.TaskHistory{}).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, d.Enqueue(ctx, msg))
	require.NoError(t, d.Recover(ctx, "core-worker", "fixture"))
	items := dequeue(t, d)
	require.Len(t, items, 1, "恢复投递不得抢占仍有效的 TaskBus 租约")
	require.ErrorIs(t, d.Complete(ctx, items[0], func(tx *gorm.DB) error {
		require.NoError(t, tx.Exec("INSERT INTO fixture_results VALUES ('rolled back')").Error)
		return rollback
	}), rollback)
	require.NoError(t, db.Table("fixture_results").Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, d.Complete(ctx, items[0], func(tx *gorm.DB) error { return tx.Exec("INSERT INTO fixture_results VALUES ('committed')").Error }))
	require.NoError(t, d.Ack(ctx, items[0]))
	history, err := d.Status(ctx, "tenant-a", "fixture", msg.ID)
	require.NoError(t, err)
	require.Equal(t, m.TaskHistoryStatusCompleted, history.Status)
	require.Equal(t, 1, history.Attempt)
	require.Equal(t, "task_driver", history.Source)
	require.NoError(t, db.Table("fixture_results").Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.False(t, db.Migrator().HasTable("media_derivative_jobs"))
}

func TestSharedHistoryReclaimFencesOldResultAndBoundsRetries(t *testing.T) {
	d, db, mini := fixture(t)
	ctx := context.Background()
	msg := message("work-crash")
	require.NoError(t, d.Enqueue(ctx, msg))
	first := dequeue(t, d)
	require.Len(t, first, 1)
	mini.FastForward(2 * time.Second)
	time.Sleep(1100 * time.Millisecond)
	second := dequeue(t, d)
	require.Len(t, second, 1)
	require.ErrorIs(t, d.Complete(ctx, first[0], func(tx *gorm.DB) error { return tx.Exec("INSERT INTO fixture_results VALUES ('stale')").Error }), event_bus.ErrLeaseStale)
	require.NoError(t, d.Reject(ctx, second[0], "TRANSIENT", false))
	time.Sleep(2 * time.Millisecond)
	third := dequeue(t, d)
	require.Len(t, third, 1)
	require.NoError(t, d.Reject(ctx, third[0], "TRANSIENT", false))
	history, err := d.Status(ctx, "tenant-a", "fixture", msg.ID)
	require.NoError(t, err)
	require.Equal(t, m.TaskHistoryStatusFailed, history.Status)
	require.Equal(t, 3, history.Attempt)
	var count int64
	require.NoError(t, db.Table("fixture_results").Count(&count).Error)
	require.Zero(t, count)
}
