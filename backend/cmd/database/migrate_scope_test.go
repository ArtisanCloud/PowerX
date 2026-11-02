package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestStorageMigrationScopeRejectsOtherCommandsAndEnvironments(t *testing.T) {
	require.NoError(t, validateMigrationScope("migrate", "storage-governance"))
	require.NoError(t, validateMigrationScope("migrate", "capability-retention"))
	require.Error(t, validateMigrationScope("seed", "capability-retention"))
	require.Error(t, validateMigrationScope("seed", "storage-governance"))
	require.Error(t, validateMigrationScope("migrate", "everything"))
	for _, env := range []string{"prod", "staging", ""} {
		for _, scope := range []string{"storage-governance", "capability-retention"} {
			err := MigrateDatabaseScope(context.Background(), nil, &config.Config{Deployment: config.DeploymentConfig{Env: env}}, scope)
			require.Error(t, err)
		}
	}
}

func TestCapabilityRetentionScopeNativePreservesLegacyRowsAndRollsBackOnFailure(t *testing.T) {
	dsn := os.Getenv("POWERX_STORAGE_TEST_DSN")
	if dsn == "" {
		t.Skip("opt-in isolated native PostgreSQL")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	for _, scenario := range []string{"preserve_and_repeat", "missing_source", "index_conflict"} {
		t.Run(scenario, func(t *testing.T) {
			schema := "retention_scope_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
			previous := coremodel.PowerXSchema
			coremodel.PowerXSchema = schema
			t.Cleanup(func() {
				require.NoError(t, db.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
				coremodel.PowerXSchema = previous
			})
			trace := schema + ".capability_registry_invocation_traces"
			event := schema + ".capability_registry_event_publications"
			require.NoError(t, db.Exec("CREATE TABLE "+trace+" (id bigint PRIMARY KEY, tenant_uuid char(36), status varchar(32), event_publication_id uuid, request_payload jsonb, sentinel text); INSERT INTO "+trace+" VALUES (1,'tenant','completed',NULL,'{\"keep\":true}','trace-preserved')").Error)
			require.NoError(t, db.Exec("CREATE TABLE "+schema+".business_sentinel(id int PRIMARY KEY, value text); INSERT INTO "+schema+".business_sentinel VALUES (1,'preserved')").Error)
			if scenario != "missing_source" {
				require.NoError(t, db.Exec("CREATE TABLE "+event+" (id bigint PRIMARY KEY, tenant_uuid char(36), status varchar(32), payload jsonb, sentinel text); INSERT INTO "+event+" VALUES (1,'tenant','pending','{\"keep\":true}','event-preserved')").Error)
			}
			if scenario == "index_conflict" {
				require.NoError(t, db.Exec("CREATE INDEX idx_capability_event_retention ON "+schema+".business_sentinel(id)").Error)
			}
			cfg := &config.Config{Deployment: config.DeploymentConfig{Env: "dev"}}
			err := MigrateDatabaseScope(context.Background(), db, cfg, "capability-retention")
			if scenario != "preserve_and_repeat" {
				require.Error(t, err)
				var columns int64
				require.NoError(t, db.Raw("SELECT count(*) FROM information_schema.columns WHERE table_schema=? AND column_name IN ('record_class','completed_at','terminal_at','delivery_receipt')", schema).Scan(&columns).Error)
				require.Zero(t, columns, "failed migration must roll back every added column")
				return
			}
			require.NoError(t, err)
			require.NoError(t, MigrateDatabaseScope(context.Background(), db, cfg, "capability-retention"))
			var columns, indexes int64
			require.NoError(t, db.Raw("SELECT count(*) FROM information_schema.columns WHERE table_schema=? AND column_name IN ('record_class','completed_at','terminal_at','delivery_receipt')", schema).Scan(&columns).Error)
			require.EqualValues(t, 5, columns)
			require.NoError(t, db.Raw("SELECT count(*) FROM pg_indexes WHERE schemaname=? AND indexname IN ('idx_capability_trace_retention','idx_capability_trace_publication','idx_capability_event_retention')", schema).Scan(&indexes).Error)
			require.EqualValues(t, 3, indexes)
			for table, wantStatus := range map[string]string{trace: "completed", event: "pending"} {
				var row struct {
					Status, RecordClass, Sentinel string
					Payload                       bool
					TerminalNull                  bool
				}
				payload, terminal := "payload", "terminal_at"
				if table == trace {
					payload, terminal = "request_payload", "completed_at"
				}
				require.NoError(t, db.Raw("SELECT status, record_class, sentinel, ("+payload+"->>'keep')::boolean AS payload, "+terminal+" IS NULL AS terminal_null FROM "+table+" WHERE id=1").Scan(&row).Error)
				require.Equal(t, wantStatus, row.Status)
				require.Equal(t, "legacy_unclassified", row.RecordClass)
				require.True(t, row.TerminalNull)
				require.True(t, row.Payload)
				require.Contains(t, row.Sentinel, "preserved")
			}
			var tables []string
			require.NoError(t, db.Raw("SELECT tablename FROM pg_tables WHERE schemaname=? ORDER BY tablename", schema).Scan(&tables).Error)
			require.Equal(t, []string{"business_sentinel", "capability_registry_event_publications", "capability_registry_invocation_traces"}, tables)
			var value string
			require.NoError(t, db.Raw("SELECT value FROM "+schema+".business_sentinel WHERE id=1").Scan(&value).Error)
			require.Equal(t, "preserved", value)
		})
	}
}

func TestStorageMigrationScopeNativeCreatesOnlyGovernanceTables(t *testing.T) {
	dsn := os.Getenv("POWERX_STORAGE_TEST_DSN")
	if dsn == "" {
		t.Skip("opt-in isolated native PostgreSQL")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	schema := "storage_scope_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = schema
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
		coremodel.PowerXSchema = previous
	})
	require.NoError(t, db.Exec("CREATE TABLE "+schema+".business_sentinel(id int PRIMARY KEY, value text); INSERT INTO "+schema+".business_sentinel VALUES (1,'preserved')").Error)
	cfg := &config.Config{Deployment: config.DeploymentConfig{Env: "dev"}}
	for i := 0; i < 2; i++ {
		require.NoError(t, MigrateDatabaseScope(context.Background(), db, cfg, "storage-governance"))
	}
	var tables []string
	require.NoError(t, db.Raw("SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname=? ORDER BY tablename", schema).Scan(&tables).Error)
	require.Equal(t, []string{"business_sentinel", "ops_storage_cleanup_runs", "ops_storage_policies", "ops_storage_policy_revisions", "ops_storage_samples"}, tables)
	var value string
	require.NoError(t, db.Raw("SELECT value FROM "+schema+".business_sentinel WHERE id=1").Scan(&value).Error)
	require.Equal(t, "preserved", value)
}
