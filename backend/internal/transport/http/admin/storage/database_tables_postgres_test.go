package storage

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	storageobserver "github.com/ArtisanCloud/PowerX/internal/infra/storage_observer"
	storagegov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	sgmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSystemTableInventoryNativePostgresReadOnly(t *testing.T) {
	dsn := os.Getenv("POWERX_TABLE_INVENTORY_TEST_DSN")
	if dsn == "" {
		t.Skip("requires an explicitly owned temporary PostgreSQL cluster")
	}
	cfg, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	require.EqualValues(t, 19464, cfg.Port)
	require.True(t, strings.HasPrefix(cfg.Host, "/tmp/pxst"))
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, _ := db.DB()
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	schema := "tables_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = schema
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
		coremodel.PowerXSchema = previous
	})
	require.NoError(t, db.AutoMigrate(&m.InvocationTrace{}, &m.HealthProbeResult{}))
	bytes := make([]byte, 65536)
	_, err = rand.Read(bytes)
	require.NoError(t, err)
	payload, _ := json.Marshal(map[string]string{"private_marker": base64.StdEncoding.EncodeToString(bytes)})
	oldest := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	newest := oldest.Add(48 * time.Hour)
	for i, at := range []time.Time{oldest, newest} {
		tr := m.InvocationTrace{TenantUUID: uuid.NewString(), TraceID: uuid.NewString(), PluginID: "owned.fixture", CapabilityID: "owned.fixture", ProtocolUsed: "fixture", Status: "completed", RequestPayload: datatypes.JSON(payload)}
		tr.CreatedAt = at
		require.NoError(t, db.Create(&tr).Error)
		if i == 1 {
			require.NoError(t, db.Delete(&tr).Error)
		}
	}
	readDB, err := gorm.Open(postgres.Open(dsn+" default_transaction_read_only=on"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	readPool, _ := readDB.DB()
	t.Cleanup(func() { require.NoError(t, readPool.Close()) })
	svc, err := storagegov.NewService(storagegov.Options{FilesystemPath: "/", Collector: testCollector{}, DatabaseCollector: storageobserver.PostgresCollector{DB: readDB}})
	require.NoError(t, err)
	w := request(t, reqctx.CoreXClaims{UserID: 1, IsRoot: true}, "GET", "/api/v1/admin/ops/storage/database-tables", "", svc)
	require.Equal(t, 200, w.Code, w.Body.String())
	var response struct {
		Data storagegov.DatabaseTableInventory `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	// 比较完整固定目录，而非已经过期的常数；新增的配额和会话表必须仍受保护。
	expected := map[string]sgmodel.SystemTableDefinition{}
	for _, definition := range sgmodel.SystemTableCatalog() {
		expected[definition.Name] = definition
	}
	require.Len(t, response.Data.Items, len(expected))
	for _, row := range response.Data.Items {
		definition, ok := expected[row.Name]
		require.True(t, ok)
		require.Equal(t, definition, row.SystemTableDefinition)
	}
	found := map[string]bool{}
	for _, row := range response.Data.Items {
		switch row.Name {
		case "capability_registry_invocation_traces":
			found[row.Name] = true
			require.Equal(t, "observed", row.State)
			require.Equal(t, "observed", row.TimeState)
			require.True(t, oldest.Equal(*row.OldestAt))
			require.True(t, newest.Equal(*row.NewestAt))
			require.Equal(t, *row.TotalBytes, *row.HeapBytes+*row.IndexesBytes+*row.ToastBytes)
			require.Greater(t, *row.ToastBytes, int64(0))
		case "capability_registry_health_probe_results":
			found[row.Name] = true
			require.Equal(t, "empty", row.TimeState)
			require.Nil(t, row.OldestAt)
		case "skills_execution_traces":
			found[row.Name] = true
			require.Equal(t, "not_present", row.State)
			require.Nil(t, row.TotalBytes)
		}
	}
	require.Len(t, found, 3)
	require.NotContains(t, w.Body.String(), "private_marker")
	var count int64
	require.NoError(t, db.WithContext(context.Background()).Unscoped().Model(&m.InvocationTrace{}).Count(&count).Error)
	require.EqualValues(t, 2, count)
}
