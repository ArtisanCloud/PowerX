package database

import (
	"os"
	"strings"
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestStorageSchemaNativeExactRelationAndKeyOrder(t *testing.T) {
	dsn := os.Getenv("POWERX_STORAGE_CONTROL_TEST_DSN")
	if dsn == "" {
		t.Skip("owned temporary PostgreSQL only")
	}
	cfg, e := pgx.ParseConfig(dsn)
	require.NoError(t, e)
	require.True(t, strings.HasPrefix(cfg.Host, "/tmp/powerx-control-owned-"))
	require.Equal(t, "storage_control_test", cfg.User)
	previous := coremodel.PowerXSchema
	defer func() { coremodel.PowerXSchema = previous }()
	for i := 0; i < 2; i++ {
		db, e := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, e)
		pool, e := db.DB()
		require.NoError(t, e)
		schema := "schema_contract_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
		t.Cleanup(func() {
			require.NoError(t, db.Exec("DROP SCHEMA IF EXISTS "+schema+" CASCADE").Error)
			require.NoError(t, pool.Close())
		})
		coremodel.PowerXSchema = schema
		require.NoError(t, MigrateStorageGovernanceModels(db))
		require.NoError(t, CheckStorageGovernanceSchema(db), "same model in another schema must not reuse an old cached relation")
		require.NoError(t, db.Exec("DROP INDEX "+schema+".uk_ops_control_request").Error)
		require.NoError(t, db.Exec("CREATE UNIQUE INDEX uk_ops_control_request ON "+schema+".ops_storage_control_revisions(tenant_id,scope_id,request_id) WHERE status=1").Error)
		requireStorageSchemaCode(t, CheckStorageGovernanceControlSchema(db), "STORAGE_CONTROL_SCHEMA_NOT_READY")
	}
}
func TestStorageSchemaZeroGormHandleIsUnavailable(t *testing.T) {
	requireStorageSchemaCode(t, CheckStorageGovernanceSchema(&gorm.DB{}), "STORAGE_GOVERNANCE_UNAVAILABLE")
}
