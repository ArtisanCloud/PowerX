package main

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	history "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func taskHistoryLegacyPostgres(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	dsn := os.Getenv("POWERX_TASK_HISTORY_TEST_DSN")
	if dsn == "" {
		t.Skip("requires disposable native PostgreSQL")
	}
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(strings.TrimPrefix(parsed.Path, "/"), "powerx_ai_media_test_"), "disposable database required")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	schema := "task_history_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
	old := core.PowerXSchema
	core.PowerXSchema = schema
	t.Cleanup(func() { require.NoError(t, db.Exec("DROP SCHEMA "+schema+" CASCADE").Error); core.PowerXSchema = old })
	// 从缺列的真实旧结构开始；保留额外 legacy 字段，防止整表重建悄悄丢数据。
	require.NoError(t, db.Exec(`CREATE TABLE `+schema+`.event_task_histories (
 id bigserial PRIMARY KEY, uuid uuid NOT NULL UNIQUE, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
 task_id varchar(191) NOT NULL, tenant_key varchar(128) NOT NULL, subscriber_id varchar(191) NOT NULL,
 topic varchar(255) NOT NULL DEFAULT '', kind varchar(128) NOT NULL DEFAULT '', source varchar(64) NOT NULL DEFAULT '', trace_id varchar(128) NOT NULL DEFAULT '',
 status varchar(32) NOT NULL DEFAULT 'pending', attempt int NOT NULL DEFAULT 0, error_message text, payload text, metadata jsonb NOT NULL DEFAULT '{}',
 submitted_at timestamptz, started_at timestamptz, completed_at timestamptz, last_seen_at timestamptz NOT NULL DEFAULT now(), legacy_sentinel text,
 CONSTRAINT uidx_event_task_histories_key UNIQUE(task_id,tenant_key,subscriber_id))`).Error)
	for _, status := range []string{"pending", "running", "completed", "failed"} {
		require.NoError(t, db.Exec(`INSERT INTO `+schema+`.event_task_histories (uuid,task_id,tenant_key,subscriber_id,status,payload,legacy_sentinel) VALUES (?,?, 'tenant','subscriber',?,'original payload','preserve me')`, uuid.NewString(), "old-"+status, status).Error)
	}
	require.NoError(t, db.Exec("CREATE TABLE "+schema+".business_sentinel(id int PRIMARY KEY, value text); INSERT INTO "+schema+".business_sentinel VALUES (1,'untouched')").Error)
	return db, schema
}
func taskHistoryFingerprint(t *testing.T, db *gorm.DB, schema string) string {
	t.Helper()
	var value string
	require.NoError(t, db.Raw(`SELECT md5(string_agg((to_jsonb(h)-'idempotency_key')::text,'|' ORDER BY id)) FROM `+schema+`.event_task_histories h`).Scan(&value).Error)
	return value
}
func TestTaskHistoryMigrationScopeNativePreservesLegacyRowsAndConstraints(t *testing.T) {
	db, schema := taskHistoryLegacyPostgres(t)
	ctx := context.Background()
	cfg := &config.Config{Deployment: config.DeploymentConfig{Env: "dev"}}
	before := taskHistoryFingerprint(t, db, schema)
	var schemaErr *database.TaskHistorySchemaError
	require.ErrorAs(t, database.CheckTaskHistorySchema(db), &schemaErr)
	require.Equal(t, "idempotency_column_missing", schemaErr.Reason)
	for i := 0; i < 2; i++ {
		require.NoError(t, MigrateDatabaseScope(ctx, db, cfg, "task-history-idempotency"))
		require.NoError(t, database.CheckTaskHistorySchema(db))
	}
	require.Equal(t, before, taskHistoryFingerprint(t, db, schema))
	var nulls int64
	require.NoError(t, db.Table(schema+".event_task_histories").Where("idempotency_key IS NULL").Count(&nulls).Error)
	require.EqualValues(t, 4, nulls)
	var tables []string
	require.NoError(t, db.Raw("SELECT tablename FROM pg_tables WHERE schemaname=? ORDER BY tablename", schema).Scan(&tables).Error)
	require.Equal(t, []string{"business_sentinel", "event_task_histories"}, tables)
	key := "exact-key"
	insert := func(task, tenant, subscriber string, k *string) error {
		return db.Create(&history.TaskHistory{TaskID: task, TenantKey: tenant, SubscriberID: subscriber, IdempotencyKey: k}).Error
	}
	require.NoError(t, insert("new-1", "tenant", "subscriber", &key))
	require.Error(t, insert("new-2", "tenant", "subscriber", &key), "exact tenant/subscriber/idempotency tuple must be unique")
	require.NoError(t, insert("new-3", "another-tenant", "subscriber", &key))
	require.NoError(t, insert("new-4", "tenant", "another-subscriber", &key))
	require.NoError(t, insert("new-null-1", "tenant", "subscriber", nil))
	require.NoError(t, insert("new-null-2", "tenant", "subscriber", nil))
	// 经过正式 Register 的写入可成功，并能与业务事务共同回滚。
	rollback := errors.New("controlled rollback")
	require.ErrorIs(t, db.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, tx.Exec("UPDATE "+schema+".business_sentinel SET value='changed'").Error)
		require.NoError(t, repo.NewTaskHistoryRepository(db).Register(ctx, tx, &history.TaskHistory{TaskID: "transaction-probe", TenantKey: "tenant", SubscriberID: "subscriber"}))
		return rollback
	}), rollback)
	var probeCount int64
	require.NoError(t, db.Table(schema+".event_task_histories").Where("task_id='transaction-probe'").Count(&probeCount).Error)
	require.Zero(t, probeCount)
	var value string
	require.NoError(t, db.Raw("SELECT value FROM "+schema+".business_sentinel WHERE id=1").Scan(&value).Error)
	require.Equal(t, "untouched", value)
}
func TestTaskHistoryMigrationScopeNativeRollsBackAndRejectsInvalidIndexes(t *testing.T) {
	for _, scenario := range []string{"wrong_columns", "name_collision", "partial_index", "duplicate_existing_keys", "wrong_column_type"} {
		t.Run(scenario, func(t *testing.T) {
			db, schema := taskHistoryLegacyPostgres(t)
			before := taskHistoryFingerprint(t, db, schema)
			switch scenario {
			case "wrong_columns":
				require.NoError(t, db.Exec("CREATE UNIQUE INDEX uidx_event_task_histories_idempotency ON "+schema+".event_task_histories(task_id)").Error)
			case "name_collision":
				require.NoError(t, db.Exec("CREATE INDEX uidx_event_task_histories_idempotency ON "+schema+".business_sentinel(id)").Error)
			case "partial_index":
				require.NoError(t, db.Exec("ALTER TABLE "+schema+".event_task_histories ADD COLUMN idempotency_key varchar(255)").Error)
				require.NoError(t, db.Exec("CREATE UNIQUE INDEX uidx_event_task_histories_idempotency ON "+schema+".event_task_histories(tenant_key,subscriber_id,idempotency_key) WHERE idempotency_key IS NOT NULL").Error)
			case "duplicate_existing_keys":
				require.NoError(t, db.Exec("ALTER TABLE "+schema+".event_task_histories ADD COLUMN idempotency_key varchar(255)").Error)
				require.NoError(t, db.Exec("UPDATE "+schema+".event_task_histories SET idempotency_key='duplicate'").Error)
			case "wrong_column_type":
				require.NoError(t, db.Exec("ALTER TABLE "+schema+".event_task_histories ADD COLUMN idempotency_key varchar(128)").Error)
			}
			err := MigrateDatabaseScope(context.Background(), db, &config.Config{Deployment: config.DeploymentConfig{Env: "test"}}, "task-history-idempotency")
			require.Error(t, err)
			require.Equal(t, before, taskHistoryFingerprint(t, db, schema))
			if scenario == "wrong_columns" || scenario == "name_collision" {
				require.False(t, db.Migrator().HasColumn(&history.TaskHistory{}, "idempotency_key"), "failed transaction must roll back the added column")
			}
			if scenario == "wrong_column_type" || scenario == "duplicate_existing_keys" {
				require.False(t, db.Migrator().HasIndex(&history.TaskHistory{}, "uidx_event_task_histories_idempotency"))
			}
		})
	}
}
