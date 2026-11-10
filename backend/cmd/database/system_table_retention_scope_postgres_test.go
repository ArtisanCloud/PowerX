package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSystemTableRetentionTargetRejectsRemoteEndpointsBeforeDatabaseAccess(t *testing.T) {
	for _, scenario := range []struct{ name, host, dsn string }{
		{"remote_host", "203.0.113.10", ""},
		{"remote_dsn_overrides_local_host", "127.0.0.1", "postgres://user@203.0.113.10/postgres"},
		{"remote_fallback", "127.0.0.1", "host=127.0.0.1,203.0.113.10 dbname=postgres"},
		{"malformed_dsn", "127.0.0.1", "://invalid"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Database.Host, cfg.Database.DSN = scenario.host, scenario.dsn
			// A nil connection proves rejection happens before any database read or DDL.
			err := validateSystemTableRetentionTarget(context.Background(), nil, cfg, "postgres", coremodel.PowerXSchema)
			require.ErrorContains(t, err, "endpoint")
		})
	}
}

func TestSystemTableRetentionScopeNativeIsNarrowAtomicAndRepeatable(t *testing.T) {
	dsn := os.Getenv("POWERX_STORAGE_TEST_DSN")
	if dsn == "" {
		t.Skip("owned temporary PostgreSQL only")
	}
	cfg, e := pgx.ParseConfig(dsn)
	require.NoError(t, e)
	require.EqualValues(t, 19464, cfg.Port)
	require.True(t, strings.HasPrefix(cfg.Host, "/tmp/pxst"))
	db, e := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool, _ := db.DB()
	defer pool.Close()
	for _, scenario := range []string{"repeat", "missing_source", "wrong_index"} {
		t.Run(scenario, func(t *testing.T) {
			schema := "typed_scope_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			previous := coremodel.PowerXSchema
			coremodel.PowerXSchema = schema
			require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
			defer func() {
				require.NoError(t, db.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
				coremodel.PowerXSchema = previous
			}()
			probe := schema + ".capability_registry_health_probe_results"
			cache := schema + ".capability_registry_discovery_cache"
			require.NoError(t, db.Exec("CREATE TABLE "+probe+" (adapter_id text,probe_window_start timestamptz,sentinel text); INSERT INTO "+probe+" VALUES ('keep',now(),'unchanged')").Error)
			require.NoError(t, db.Exec("CREATE TABLE "+schema+".business_sentinel(id int PRIMARY KEY,value text); INSERT INTO "+schema+".business_sentinel VALUES(1,'preserved')").Error)
			if scenario != "missing_source" {
				require.NoError(t, db.Exec("CREATE TABLE "+cache+" (expires_at timestamptz,sentinel text); INSERT INTO "+cache+" VALUES (now(),'unchanged')").Error)
			}
			if scenario == "wrong_index" {
				require.NoError(t, db.Exec("CREATE INDEX idx_registry_discovery_expiry ON "+schema+".business_sentinel(id)").Error)
			}
			c := &config.Config{Deployment: config.DeploymentConfig{Env: "dev"}}
			c.Database.DSN = dsn
			require.NoError(t, db.Raw("SELECT 'pg-' || system_identifier::text FROM pg_control_system()").Scan(&c.StorageGovernance.PostgresClusterID).Error)
			require.NoError(t, validateSystemTableRetentionTarget(context.Background(), db, c, "postgres", schema))
			require.Error(t, validateSystemTableRetentionTarget(context.Background(), db, c, "wrong", schema))
			require.Error(t, validateSystemTableRetentionTarget(context.Background(), db, c, "postgres", "wrong"))
			cluster := c.StorageGovernance.PostgresClusterID
			c.StorageGovernance.PostgresClusterID = "pg-wrong"
			require.ErrorContains(t, validateSystemTableRetentionTarget(context.Background(), db, c, "postgres", schema), "cluster identity mismatch")
			c.StorageGovernance.PostgresClusterID = cluster
			err := MigrateDatabaseScope(context.Background(), db, c, "system-table-retention")
			if scenario != "repeat" {
				require.Error(t, err)
				var n int64
				require.NoError(t, db.Raw("SELECT count(*) FROM pg_indexes WHERE schemaname=? AND indexname='idx_registry_probe_retention'", schema).Scan(&n).Error)
				require.Zero(t, n)
			} else {
				require.NoError(t, err)
				require.NoError(t, MigrateDatabaseScope(context.Background(), db, c, "system-table-retention"))
				var n int64
				require.NoError(t, db.Raw("SELECT count(*) FROM pg_indexes WHERE schemaname=? AND indexname IN ('idx_registry_probe_retention','idx_registry_discovery_expiry')", schema).Scan(&n).Error)
				require.EqualValues(t, 2, n)
			}
			var value string
			require.NoError(t, db.Raw("SELECT value FROM "+schema+".business_sentinel WHERE id=1").Scan(&value).Error)
			require.Equal(t, "preserved", value)
			require.NoError(t, db.Raw("SELECT sentinel FROM "+probe).Scan(&value).Error)
			require.Equal(t, "unchanged", value)
		})
	}
}
