package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	nm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/notification"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func notificationReminderScopeDB(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	dsn := os.Getenv("POWERX_NOTIFICATION_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("owned temporary PostgreSQL only")
	}
	connection, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(connection.Host, "/tmp/powerx-notification-scope-"))
	require.Equal(t, "notification_scope_test", connection.User)
	require.Equal(t, "notification_scope_test", connection.Database)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	schema := "notification_scope_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = schema
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
		coremodel.PowerXSchema = previous
		require.NoError(t, pool.Close())
	})
	return db, schema
}

func migrateNotificationReminderScope(db *gorm.DB) error {
	return MigrateDatabaseScope(context.Background(), db, &config.Config{Deployment: config.DeploymentConfig{Env: "test"}}, "notification-reminder-dependencies")
}

func TestNotificationReminderScopeNativePreservesDataAndIsIdempotent(t *testing.T) {
	db, schema := notificationReminderScopeDB(t)
	member, tenant := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Exec("CREATE TABLE "+schema+".iam_member(uuid uuid PRIMARY KEY,tenant_uuid uuid,sentinel jsonb NOT NULL)").Error)
	require.NoError(t, db.Exec("INSERT INTO "+schema+".iam_member VALUES (?,?,'{\"keep\":true}')", member, tenant).Error)
	require.NoError(t, db.Exec("CREATE TABLE "+schema+".system_notifications(uuid uuid PRIMARY KEY,sentinel text)").Error)
	require.NoError(t, db.Exec("INSERT INTO "+schema+".system_notifications VALUES (?,'notification-preserved')", uuid.New()).Error)
	require.NoError(t, db.Exec("CREATE TABLE "+schema+".business_sentinel(uuid uuid PRIMARY KEY,sentinel text)").Error)
	require.NoError(t, db.Exec("INSERT INTO "+schema+".business_sentinel VALUES (?,'business-preserved')", uuid.New()).Error)
	var original string
	require.NoError(t, db.Raw("SELECT to_jsonb(m)::text FROM "+schema+".iam_member m").Scan(&original).Error)
	require.NoError(t, migrateNotificationReminderScope(db))
	var preserved string
	require.NoError(t, db.Raw("SELECT (to_jsonb(m)-'direct_manager_member_uuid')::text FROM "+schema+".iam_member m").Scan(&preserved).Error)
	require.Equal(t, original, preserved)
	var nullManager bool
	require.NoError(t, db.Raw("SELECT direct_manager_member_uuid IS NULL FROM "+schema+".iam_member").Scan(&nullManager).Error)
	require.True(t, nullManager)
	var count int64
	require.NoError(t, db.Model(&nm.CreationReceipt{}).Count(&count).Error)
	require.Zero(t, count, "migration must not fabricate historical creation receipts")
	var tables []string
	require.NoError(t, db.Raw("SELECT tablename FROM pg_tables WHERE schemaname=? ORDER BY tablename", schema).Scan(&tables).Error)
	require.Equal(t, []string{"business_sentinel", "iam_member", "system_notification_creation_receipts", "system_notifications"}, tables)
	var columns []string
	require.NoError(t, db.Raw("SELECT column_name FROM information_schema.columns WHERE table_schema=? AND table_name='iam_member' ORDER BY ordinal_position", schema).Scan(&columns).Error)
	require.Equal(t, []string{"uuid", "tenant_uuid", "sentinel", "direct_manager_member_uuid"}, columns, "member AutoMigrate would add unrelated columns")

	// 迁移再次执行时保留已明确配置的上级及实际创建凭据。
	manager := uuid.NewString()
	require.NoError(t, db.Exec("UPDATE "+schema+".iam_member SET direct_manager_member_uuid=?", manager).Error)
	receipt := nm.CreationReceipt{UUID: uuid.New(), TenantUUID: tenant, ServiceOwner: "plugin.fixture", KeyHash: strings.Repeat("a", 64), RequestHash: strings.Repeat("b", 64), NotificationUUID: uuid.New(), ResponseJSON: datatypes.JSON(`{"original":true}`)}
	require.NoError(t, db.Create(&receipt).Error)
	require.NoError(t, migrateNotificationReminderScope(db))
	var actualManager string
	require.NoError(t, db.Raw("SELECT direct_manager_member_uuid FROM "+schema+".iam_member").Scan(&actualManager).Error)
	require.Equal(t, manager, actualManager)
	var found nm.CreationReceipt
	require.NoError(t, db.First(&found, "uuid=?", receipt.UUID).Error)
	require.Equal(t, receipt.UUID, found.UUID)
	require.Equal(t, receipt.RequestHash, found.RequestHash)
	require.Equal(t, receipt.NotificationUUID, found.NotificationUUID)
	require.JSONEq(t, string(receipt.ResponseJSON), string(found.ResponseJSON))
	duplicate := receipt
	duplicate.UUID = uuid.New()
	require.Error(t, db.Create(&duplicate).Error, "same tenant, service owner and key must be unique")
	duplicate.UUID, duplicate.ServiceOwner = uuid.New(), "plugin.other"
	require.NoError(t, db.Create(&duplicate).Error)
	duplicate.UUID, duplicate.TenantUUID, duplicate.ServiceOwner = uuid.New(), uuid.NewString(), receipt.ServiceOwner
	require.NoError(t, db.Create(&duplicate).Error)
	require.NoError(t, migrateNotificationReminderScope(db))
	require.NoError(t, db.Model(&nm.CreationReceipt{}).Count(&count).Error)
	require.EqualValues(t, 3, count)
	for table, want := range map[string]string{"system_notifications": "notification-preserved", "business_sentinel": "business-preserved"} {
		var value string
		require.NoError(t, db.Raw("SELECT sentinel FROM "+schema+"."+table).Scan(&value).Error)
		require.Equal(t, want, value)
	}
}

func TestNotificationReminderScopeNativeRejectsIncompatibleSourcesAndRollsBack(t *testing.T) {
	for _, scenario := range []string{"missing_member", "missing_notifications", "manager_type", "manager_index_collision", "receipt_index_collision", "receipt_nonunique_index", "receipt_reordered_index", "receipt_column_type"} {
		t.Run(scenario, func(t *testing.T) {
			db, schema := notificationReminderScopeDB(t)
			if scenario != "missing_member" {
				require.NoError(t, db.Exec("CREATE TABLE "+schema+".iam_member(uuid uuid PRIMARY KEY)").Error)
			}
			if scenario != "missing_notifications" {
				require.NoError(t, db.Exec("CREATE TABLE "+schema+".system_notifications(uuid uuid PRIMARY KEY)").Error)
			}
			require.NoError(t, db.Exec("CREATE TABLE "+schema+".business_sentinel(uuid uuid PRIMARY KEY)").Error)
			if scenario == "manager_type" {
				require.NoError(t, db.Exec("ALTER TABLE "+schema+".iam_member ADD COLUMN direct_manager_member_uuid text").Error)
			}
			if scenario == "manager_index_collision" {
				statement := &gorm.Statement{DB: db}
				require.NoError(t, statement.Parse(&im.Member{}))
				index := statement.Schema.LookIndex("DirectManagerMemberUUID")
				require.NotNil(t, index)
				require.NoError(t, db.Exec("CREATE INDEX "+index.Name+" ON "+schema+".business_sentinel(uuid)").Error)
			}
			if scenario == "receipt_index_collision" {
				require.NoError(t, db.Exec("CREATE INDEX ux_notification_creation_scope ON "+schema+".business_sentinel(uuid)").Error)
			}
			existingReceipt := strings.HasPrefix(scenario, "receipt_") && scenario != "receipt_index_collision"
			if existingReceipt {
				require.NoError(t, db.Migrator().CreateTable(&nm.CreationReceipt{}))
				require.NoError(t, db.Exec("INSERT INTO "+schema+".system_notification_creation_receipts VALUES (?,?,?, ?,?,?,?)", uuid.New(), uuid.New(), "plugin.fixture", strings.Repeat("a", 64), strings.Repeat("b", 64), uuid.New(), `{"original":true}`).Error)
				switch scenario {
				case "receipt_column_type":
					require.NoError(t, db.Exec("ALTER TABLE "+schema+".system_notification_creation_receipts ALTER COLUMN key_hash TYPE text").Error)
				default:
					require.NoError(t, db.Exec("DROP INDEX "+schema+".ux_notification_creation_scope").Error)
					definition := "CREATE INDEX ux_notification_creation_scope ON " + schema + ".system_notification_creation_receipts(tenant_uuid,service_owner,key_hash)"
					if scenario == "receipt_reordered_index" {
						definition = "CREATE UNIQUE INDEX ux_notification_creation_scope ON " + schema + ".system_notification_creation_receipts(service_owner,tenant_uuid,key_hash)"
					}
					require.NoError(t, db.Exec(definition).Error)
				}
			}
			require.Error(t, migrateNotificationReminderScope(db))
			var managerColumns int64
			require.NoError(t, db.Raw("SELECT count(*) FROM information_schema.columns WHERE table_schema=? AND table_name='iam_member' AND column_name='direct_manager_member_uuid'", schema).Scan(&managerColumns).Error)
			if scenario == "manager_type" {
				require.EqualValues(t, 1, managerColumns)
			} else {
				require.Zero(t, managerColumns, "failed migration must roll back the added manager column")
			}
			require.Equal(t, existingReceipt, db.Migrator().HasTable(&nm.CreationReceipt{}))
			if existingReceipt {
				var count int64
				require.NoError(t, db.Model(&nm.CreationReceipt{}).Count(&count).Error)
				require.EqualValues(t, 1, count, "incompatible existing receipts must remain untouched")
			}
		})
	}
}
