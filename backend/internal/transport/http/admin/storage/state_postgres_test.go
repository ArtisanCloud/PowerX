package storage

import (
	"context"
	"encoding/json"
	"fmt"
	storageobserver "github.com/ArtisanCloud/PowerX/internal/infra/storage_observer"
	storagegov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// 仅使用显式测试 DSN；验收运行于 Agent 自己启动的临时 PG 集群，绝不回退用户配置。
func storageNativeDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("POWERX_STORAGE_TEST_DSN")
	if dsn == "" {
		t.Skip("opt-in temporary native PostgreSQL")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	connection, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, connection.Close()) })
	schema := "storage_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = schema
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
		coremodel.PowerXSchema = previous
	})
	require.NoError(t, db.AutoMigrate(&m.Policy{}, &m.PolicyRevision{}, &m.Sample{}, &m.CleanupRun{}))
	return db
}

func TestStorageNativePolicyCASAuditRollbackAndRoute(t *testing.T) {
	db := storageNativeDB(t)
	s, err := storagegov.NewService(storagegov.Options{DB: db, Identity: storagegov.Identity{HostID: "host", FilesystemID: "fs", InstanceID: "test-install", Environment: "dev"}, FilesystemPath: "/", Collector: testCollector{}})
	require.NoError(t, err)
	ctx := reqctx.WithIsRoot(context.Background(), true)
	ctx = reqctx.WithClaims(ctx, &reqctx.CoreXClaims{UserID: 1, IsRoot: true})
	in := storagegov.PolicyInput{SchemaVersion: 1, State: "draft", Budget: storagegov.HostBudget{MaxManagedBytes: 90 << 30, MinFreeBytes: 20 << 30, CriticalFreeBytes: 10 << 30}}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := s.SavePolicy(ctx, 1, in); results <- e }()
	}
	wg.Wait()
	close(results)
	wins, conflicts := 0, 0
	for err := range results {
		if err == nil {
			wins++
		} else if err == storagegov.ErrPolicyConflict {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	require.Equal(t, 1, wins)
	require.Equal(t, 1, conflicts)
	var auditCount int64
	require.NoError(t, db.Model(&m.PolicyRevision{}).Count(&auditCount).Error)
	require.EqualValues(t, 1, auditCount)
	root := reqctx.CoreXClaims{UserID: 1, IsRoot: true}
	read := request(t, root, "GET", "/api/v1/admin/ops/storage/policy", "", s)
	require.Equal(t, 200, read.Code)
	require.Contains(t, read.Body.String(), `"revision":1`)
	body := `{"schema_version":1,"expected_revision":1,"state":"draft","budget":{"max_managed_bytes":96636764160,"min_free_bytes":21474836480,"critical_free_bytes":10737418240},"resources":[]}`
	write := request(t, root, "PUT", "/api/v1/admin/ops/storage/policy", body, s)
	require.Equal(t, 200, write.Code, write.Body.String())
	require.Equal(t, 409, request(t, root, "PUT", "/api/v1/admin/ops/storage/policy", body, s).Code)
	for _, claims := range []reqctx.CoreXClaims{{UserID: 1}, {UserID: 1, IsRoot: true, Platforms: []string{"service_actor"}}, {UserID: 1, IsRoot: true, Platforms: []string{"api_key"}}} {
		for _, path := range []string{"policy", "history"} {
			require.Equal(t, 403, request(t, claims, "GET", "/api/v1/admin/ops/storage/"+path, "", s).Code)
		}
		require.Equal(t, 403, request(t, claims, "PUT", "/api/v1/admin/ops/storage/policy", body, s).Code)
	}
	require.Equal(t, 400, request(t, root, "PUT", "/api/v1/admin/ops/storage/policy", strings.TrimSuffix(body, "}")+`,"path":"/etc"}`, s).Code)
	// 审计表故障时 CAS 更新必须回滚。
	require.NoError(t, db.Migrator().DropTable(&m.PolicyRevision{}))
	in.ExpectedRevision = 2
	_, err = s.SavePolicy(ctx, 1, in)
	require.ErrorIs(t, err, storagegov.ErrStateUnavailable)
	readback, err := s.Policy(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 2, readback.Revision)
	// 观测持久化故障不能成为默认零策略或正常历史。
	require.NoError(t, db.Migrator().DropTable(&m.Policy{}))
	require.Equal(t, 503, request(t, root, "GET", "/api/v1/admin/ops/storage/policy", "", s).Code)
}

func TestStorageNativeSamplesBoundedIdempotentAndInstanceScoped(t *testing.T) {
	db := storageNativeDB(t)
	now := time.Now().UTC().Truncate(5 * time.Minute)
	s, err := storagegov.NewService(storagegov.Options{DB: db, Identity: storagegov.Identity{InstanceID: "owned-instance", Environment: "dev"}, FilesystemPath: "/", Collector: testCollector{}, Now: func() time.Time { return now }})
	require.NoError(t, err)
	ctx := context.Background()
	r := repo.NewStateRepository(db)
	require.NoError(t, r.SaveSample(ctx, &m.Sample{InstanceID: "owned-instance", Slot: now.Add(-16 * 24 * time.Hour), MetricVersion: "old", Document: []byte(`{}`)}))
	require.NoError(t, r.SaveSample(ctx, &m.Sample{InstanceID: "other-instance", Slot: now.Add(-5 * time.Minute), MetricVersion: "other", Document: []byte(`{}`)}))
	for i := 0; i < 3; i++ {
		require.NoError(t, s.CaptureSample(ctx))
	}
	var owned int64
	require.NoError(t, db.Model(&m.Sample{}).Where("instance_id = ?", "owned-instance").Count(&owned).Error)
	require.EqualValues(t, 1, owned)
	var others int64
	require.NoError(t, db.Model(&m.Sample{}).Where("instance_id = ?", "other-instance").Count(&others).Error)
	require.EqualValues(t, 1, others)
	now = now.Add(5 * time.Minute)
	history, err := s.History(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, history.SampleCount)
	require.Nil(t, history.DailyIncrementBytes)
	body, _ := json.Marshal(history)
	require.NotContains(t, string(body), "other-instance")
}

func TestStorageNativePostgresMetricsAndDeniedWAL(t *testing.T) {
	db := storageNativeDB(t)
	schema := coremodel.PowerXSchema
	require.NoError(t, db.Exec("CREATE TABLE "+schema+".large_relation (id int PRIMARY KEY, payload text)").Error)
	require.NoError(t, db.Exec("ALTER TABLE "+schema+".large_relation ALTER COLUMN payload SET STORAGE EXTERNAL").Error)
	require.NoError(t, db.Exec("INSERT INTO "+schema+".large_relation SELECT i, repeat(md5(i::text), 400) FROM generate_series(1,200) i").Error)
	collector := storageobserver.PostgresCollector{DB: db, Identity: storagegov.Identity{InstanceID: "dev-instance", Environment: "dev"}, ClusterID: "shared-pg", Relations: []storageobserver.RelationSource{{ID: "test-table", Schema: schema, Name: "large_relation"}}}
	out := collector.CollectDatabase(context.Background())
	require.Equal(t, "observed", out.State)
	require.NotNil(t, out.DatabaseBytes)
	require.NotNil(t, out.WALBytes)
	require.Nil(t, out.ReusableBytes)
	require.Len(t, out.Relations, 1)
	table := out.Relations[0]
	require.Positive(t, *table.HeapBytes)
	require.Positive(t, *table.IndexesBytes)
	require.Positive(t, *table.ToastBytes)
	require.Equal(t, *table.TotalBytes, *table.HeapBytes+*table.IndexesBytes+*table.ToastBytes)
	role := "storage_reader_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	require.NoError(t, db.Exec("CREATE ROLE "+role).Error)
	t.Cleanup(func() { require.NoError(t, db.Exec("DROP ROLE "+role).Error) })
	require.Error(t, db.Transaction(func(tx *gorm.DB) error {
		if e := tx.Exec("SET LOCAL ROLE " + role).Error; e != nil {
			return e
		}
		// 每个查询使用独立事务外连接才可在权限错误后继续；此处 WAL 最后执行。
		limited := collector
		limited.DB = tx
		limited.Relations = nil
		denied := limited.CollectDatabase(context.Background())
		require.Nil(t, denied.WALBytes)
		require.Equal(t, "unavailable", denied.WALState)
		require.NotEqual(t, "observed", denied.State)
		return fmt.Errorf("rollback test role")
	}))
}
