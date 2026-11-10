package database

import (
	"fmt"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"strings"
	"testing"
)

func TestSchedulerDispatchMigrationPreservesRunsAndIsNarrow(t *testing.T) {
	old := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = old })
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE scheduler_job_runs (id integer PRIMARY KEY,status text,sentinel text); INSERT INTO scheduler_job_runs VALUES (1,'triggered','preserved')`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE business_sentinel (id integer PRIMARY KEY,value text); INSERT INTO business_sentinel VALUES (1,'untouched')`).Error)
	require.NoError(t, MigrateSchedulerTaskDispatch(db))
	require.NoError(t, MigrateSchedulerTaskDispatch(db))
	var row struct {
		Status, Sentinel    string
		DispatchAttempts    int
		NextDispatchAt      *string
		DispatchPayloadJSON *string
	}
	require.NoError(t, db.Table("scheduler_job_runs").First(&row).Error)
	require.Equal(t, "triggered", row.Status)
	require.Equal(t, "preserved", row.Sentinel)
	require.Zero(t, row.DispatchAttempts)
	require.Nil(t, row.NextDispatchAt)
	require.Nil(t, row.DispatchPayloadJSON)
	columns, err := db.Migrator().ColumnTypes("scheduler_job_runs")
	require.NoError(t, err)
	require.Len(t, columns, 6)
	var value string
	require.NoError(t, db.Raw("SELECT value FROM business_sentinel").Scan(&value).Error)
	require.Equal(t, "untouched", value)
}

func TestSchedulerDispatchMigrationNativePreservesLegacy(t *testing.T) {
	dsn := os.Getenv("POWERX_SCHEDULER_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated native PostgreSQL opt in")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	schema := "scheduler_dispatch_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
	old := coremodel.PowerXSchema
	coremodel.PowerXSchema = schema
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
		coremodel.PowerXSchema = old
	})
	table := schema + "." + coremodel.TableSchedulerJobRuns
	require.NoError(t, db.Exec("CREATE TABLE "+table+" (id bigint PRIMARY KEY,status text,sentinel text); INSERT INTO "+table+" VALUES (1,'triggered','preserved')").Error)
	require.NoError(t, db.Exec("CREATE TABLE "+schema+".business_sentinel(id int PRIMARY KEY,value text); INSERT INTO "+schema+".business_sentinel VALUES (1,'untouched')").Error)
	require.NoError(t, MigrateSchedulerTaskDispatch(db))
	require.NoError(t, MigrateSchedulerTaskDispatch(db))
	var status, sentinel string
	require.NoError(t, db.Raw("SELECT status FROM "+table+" WHERE id=1").Scan(&status).Error)
	require.Equal(t, "triggered", status)
	require.NoError(t, db.Raw("SELECT sentinel FROM "+table+" WHERE id=1").Scan(&sentinel).Error)
	require.Equal(t, "preserved", sentinel)
	var count int64
	require.NoError(t, db.Raw("SELECT count(*) FROM information_schema.columns WHERE table_schema=? AND table_name=?", schema, coremodel.TableSchedulerJobRuns).Scan(&count).Error)
	require.EqualValues(t, 6, count)
	var tables []string
	require.NoError(t, db.Raw("SELECT tablename FROM pg_tables WHERE schemaname=?", schema).Scan(&tables).Error)
	require.ElementsMatch(t, []string{"business_sentinel", coremodel.TableSchedulerJobRuns}, tables)
}
