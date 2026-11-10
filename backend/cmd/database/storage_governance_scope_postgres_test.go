package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestStorageGovernanceScopeNativeRepeatAndRollback(t *testing.T) {
	dsn := os.Getenv("POWERX_STORAGE_CONTROL_TEST_DSN")
	if dsn == "" {
		t.Skip("requires owned temporary PostgreSQL")
	}
	parsed, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(parsed.Host, "/tmp/powerx-control-owned-"))
	require.Equal(t, "storage_control_test", parsed.User)
	for _, scenario := range []string{"repeat", "rollback"} {
		t.Run(scenario, func(t *testing.T) {
			db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			pool, err := db.DB()
			require.NoError(t, err)
			schema := "storage_scope_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			old := coremodel.PowerXSchema
			coremodel.PowerXSchema = schema
			require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
			t.Cleanup(func() {
				require.NoError(t, db.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
				coremodel.PowerXSchema = old
				_ = pool.Close()
			})
			require.NoError(t, db.Exec("CREATE TABLE "+schema+".business_sentinel(id int primary key, value text); INSERT INTO "+schema+".business_sentinel VALUES (1,'preserved')").Error)
			if scenario == "rollback" {
				require.NoError(t, db.Exec("CREATE INDEX uk_ops_control_request ON "+schema+".business_sentinel(id)").Error)
			}
			cfg := &config.Config{Deployment: config.DeploymentConfig{Env: "dev"}}
			err = MigrateDatabaseScope(context.Background(), db, cfg, "storage-governance")
			if scenario == "rollback" {
				require.Error(t, err)
				require.False(t, db.Migrator().HasTable(&m.ControlConfiguration{}), "failed scope migration must roll back new tables")
			} else {
				require.NoError(t, err)
				require.NoError(t, MigrateDatabaseScope(context.Background(), db, cfg, "storage-governance"))
				require.NoError(t, database.CheckStorageGovernanceSchema(db))
				var rows int64
				require.NoError(t, db.Model(&m.ControlConfiguration{}).Count(&rows).Error)
				require.Zero(t, rows, "schema migration must not adopt or enable policies")
				require.False(t, db.Migrator().HasTable(schema+".iam_users"), "scope must not migrate IAM")
			}
			var value string
			require.NoError(t, db.Raw("SELECT value FROM "+schema+".business_sentinel WHERE id=1").Scan(&value).Error)
			require.Equal(t, "preserved", value)
		})
	}
}
