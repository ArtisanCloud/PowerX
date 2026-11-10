package database

import (
	"context"
	"os"
	"strings"
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func storageSchemaSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	old := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = pool.Close(); coremodel.PowerXSchema = old })
	return db
}

func requireStorageSchemaCode(t *testing.T, err error, code string) {
	t.Helper()
	var problem *StorageGovernanceSchemaError
	require.ErrorAs(t, err, &problem)
	require.Equal(t, code, problem.ReasonCode())
}

func TestStorageSchemaReadOnlyMissingAndUnavailable(t *testing.T) {
	db := storageSchemaSQLite(t)
	requireStorageSchemaCode(t, CheckStorageGovernanceSchema(db), "STORAGE_CONTROL_SCHEMA_NOT_READY")
	require.False(t, db.Migrator().HasTable(&m.ControlConfiguration{}), "readiness must never create a table")
	pool, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, pool.Close())
	requireStorageSchemaCode(t, CheckStorageGovernanceSchema(db), "STORAGE_GOVERNANCE_UNAVAILABLE")
	requireStorageSchemaCode(t, CheckStorageGovernanceSchema(nil), "STORAGE_GOVERNANCE_UNAVAILABLE")
}

func TestStorageSchemaChecksFieldsExactUniqueIndexesAndAllSixTables(t *testing.T) {
	for _, scenario := range []string{"missing_column", "missing_index", "nonunique_index", "wrong_columns", "missing_other_table"} {
		t.Run(scenario, func(t *testing.T) {
			db := storageSchemaSQLite(t)
			require.NoError(t, MigrateStorageGovernanceModels(db))
			require.NoError(t, MigrateStorageGovernanceModels(db))
			switch scenario {
			case "missing_column":
				require.NoError(t, db.Exec("ALTER TABLE main.ops_storage_control_configurations DROP COLUMN applied_revision").Error)
			case "missing_other_table":
				require.NoError(t, db.Migrator().DropTable(&m.CleanupRun{}))
				require.NoError(t, CheckStorageGovernanceControlSchema(db))
			default:
				require.NoError(t, db.Exec("DROP INDEX main.uk_ops_control_request").Error)
				if scenario == "nonunique_index" {
					require.NoError(t, db.Exec("CREATE INDEX main.uk_ops_control_request ON ops_storage_control_revisions(tenant_id,scope_id,request_id)").Error)
				}
				if scenario == "wrong_columns" {
					require.NoError(t, db.Exec("CREATE UNIQUE INDEX main.uk_ops_control_request ON ops_storage_control_revisions(tenant_id,request_id)").Error)
				}
			}
			requireStorageSchemaCode(t, CheckStorageGovernanceSchema(db), "STORAGE_CONTROL_SCHEMA_NOT_READY")
		})
	}
}

func TestStorageSchemaPostgresPartialIndexAndReadFailure(t *testing.T) {
	dsn := os.Getenv("POWERX_STORAGE_CONTROL_TEST_DSN")
	if dsn == "" {
		t.Skip("requires owned temporary PostgreSQL")
	}
	cfg, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(cfg.Host, "/tmp/powerx-control-owned-"))
	require.Equal(t, "storage_control_test", cfg.User)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	schema := "storage_schema_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	old := coremodel.PowerXSchema
	coremodel.PowerXSchema = schema
	require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() { coremodel.PowerXSchema = old; _ = pool.Close() })
	require.NoError(t, MigrateStorageGovernanceModels(db))
	require.NoError(t, MigrateStorageGovernanceModels(db))
	require.NoError(t, db.Exec("DROP INDEX "+schema+".uk_ops_control_request").Error)
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX uk_ops_control_request ON "+schema+".ops_storage_control_revisions(tenant_id,scope_id,request_id) WHERE status=1").Error)
	requireStorageSchemaCode(t, CheckStorageGovernanceSchema(db), "STORAGE_CONTROL_SCHEMA_NOT_READY")
	require.NoError(t, db.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
	require.NoError(t, pool.Close())
	requireStorageSchemaCode(t, CheckStorageGovernanceSchema(db.WithContext(context.Background())), "STORAGE_GOVERNANCE_UNAVAILABLE")
}
