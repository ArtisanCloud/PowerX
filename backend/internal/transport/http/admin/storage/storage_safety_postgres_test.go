package storage

import (
	"context"
	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	observer "github.com/ArtisanCloud/PowerX/internal/infra/storage_observer"
	gov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	tenants "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStorageSafetyNativePostgres(t *testing.T) {
	dsn := os.Getenv("POWERX_STORAGE_TEST_DSN")
	if dsn == "" {
		t.Skip("explicitly owned temporary PostgreSQL only")
	}
	cfg, e := pgx.ParseConfig(dsn)
	require.NoError(t, e)
	require.EqualValues(t, 19464, cfg.Port)
	require.True(t, strings.HasPrefix(cfg.Host, "/tmp/pxst"))
	db := storageNativeDB(t)
	require.NoError(t, db.AutoMigrate(&m.HealthProbeResult{}, &m.DiscoveryCacheEntry{}, &tenants.Tenant{}))
	now := time.Now().UTC()
	old := now.AddDate(0, 0, -45)
	cutoff := now.AddDate(0, 0, -30)
	probes := []m.HealthProbeResult{
		{AdapterID: "primary", ProbeWindowStart: old, Status: "healthy", NextRetryAt: old, CreatedAt: old, UpdatedAt: old},
		{AdapterID: "primary", ProbeWindowStart: now, Status: "healthy", NextRetryAt: now, CreatedAt: now, UpdatedAt: now},
		{AdapterID: "primary", ProbeWindowStart: old.Add(time.Hour), Status: "pending", NextRetryAt: old, CreatedAt: old, UpdatedAt: old},
		{AdapterID: "primary", ProbeWindowStart: old.Add(2 * time.Hour), Status: "unhealthy", NextRetryAt: now.Add(time.Hour), CreatedAt: old, UpdatedAt: old},
		{AdapterID: "only-current", ProbeWindowStart: old, Status: "healthy", NextRetryAt: old, CreatedAt: old, UpdatedAt: old},
	}
	require.NoError(t, db.Create(&probes).Error)
	tenant := &tenants.Tenant{Key: "owned-safety-test", Name: "Owned safety test"}
	require.NoError(t, db.Create(tenant).Error)
	caches := []m.DiscoveryCacheEntry{}
	for _, key := range []string{"retired", "active-fallback", "future-expiry", "unknown-tenant"} {
		row := m.DiscoveryCacheEntry{TenantUUID: tenant.UUID.String(), CapabilityID: key, ClientID: "owned", SnapshotVersion: 1, PayloadHash: "fixture", ExpiresAt: old, IssuedAt: old, Source: "fixture", CreatedAt: old, UpdatedAt: old, DeletedAt: gorm.DeletedAt{Time: old, Valid: true}}
		if key == "active-fallback" {
			row.DeletedAt = gorm.DeletedAt{}
		}
		if key == "future-expiry" {
			row.ExpiresAt = now.Add(time.Hour)
		}
		if key == "unknown-tenant" {
			row.TenantUUID = uuid.NewString()
		}
		caches = append(caches, row)
	}
	require.NoError(t, db.Create(&caches).Error)
	r := repo.NewSystemRetentionRepository(db)
	for _, id := range []string{repo.ProbeRetentionID, repo.DiscoveryRetentionID} {
		n, e := r.Batch(context.Background(), id, cutoff, now, 200, false)
		require.NoError(t, e)
		require.EqualValues(t, 1, n)
	}
	// 行锁竞争跳过，既不等待业务锁也不删除未取得锁的候选。
	locked := db.Begin()
	require.NoError(t, locked.Error)
	require.NoError(t, locked.Exec("SELECT 1 FROM "+m.HealthProbeResult{}.TableName()+" WHERE adapter_id=? AND probe_window_start=? FOR UPDATE", "primary", old).Error)
	n, e := r.Batch(context.Background(), repo.ProbeRetentionID, cutoff, now, 200, true)
	require.NoError(t, e)
	require.Zero(t, n)
	require.NoError(t, locked.Rollback().Error)

	dir := t.TempDir()
	coordinator, e := budget.New(budget.Config{Directory: filepath.Join(dir, "state"), FilesystemPath: dir, HostID: "owned-host", FilesystemID: "owned-disk", InstanceID: "owned-safety", MinFreeBytes: 1 << 20, CriticalFreeBytes: 1})
	require.NoError(t, e)
	svc, e := gov.NewService(gov.Options{DB: db, Identity: gov.Identity{HostID: "owned-host", FilesystemID: "owned-disk", InstanceID: "owned-safety", Environment: "test"}, Collector: testCollector{}, FilesystemPath: dir, Budget: coordinator, DatabaseRetentionAllowed: true, LifecycleWorkerEnabled: true, DatabaseCollector: observer.PostgresCollector{DB: db, ClusterID: "owned-fixture"}, SamplingConfigured: true, SamplingWorkerEnabled: true, Now: func() time.Time { return now }})
	require.NoError(t, e)
	root := reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access"}
	ctx := reqctx.WithIsRoot(reqctx.WithClaims(context.Background(), &root), true)
	path := "/api/v1/admin/ops/storage/database-tables/" + repo.ProbeRetentionID + "/retention"
	w := request(t, root, "GET", path, "", svc)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"enabled":false`)
	body := `{"expected_revision":0,"confirm":true,"policy":{"enabled":true,"retention_days":30,"batch_limit":200}}`
	require.Equal(t, 428, request(t, root, "PUT", path, body, svc).Code, "legacy writer must not bypass database authority")
	w = request(t, root, "POST", path+"/previews", `{"expected_revision":0}`, svc)
	require.Equal(t, 503, w.Code, "a standalone legacy service cannot clean without authority")
	var count int64
	require.NoError(t, db.Unscoped().Model(&m.HealthProbeResult{}).Count(&count).Error)
	require.EqualValues(t, 5, count)
	n, e = r.Batch(ctx, repo.DiscoveryRetentionID, cutoff, now, 200, true)
	require.NoError(t, e)
	require.EqualValues(t, 1, n)
	require.NoError(t, db.Unscoped().Model(&m.DiscoveryCacheEntry{}).Count(&count).Error)
	require.EqualValues(t, 3, count)
	// 未建索引、外部引用和未知表全部阻止执行。
	require.NoError(t, db.Exec("DROP INDEX "+strings.Split(m.DiscoveryCacheEntry{}.TableName(), ".")[0]+".idx_registry_discovery_expiry").Error)
	require.ErrorContains(t, r.Ready(ctx, repo.DiscoveryRetentionID), "index_required")
	require.NoError(t, db.Exec("CREATE TABLE "+m.HealthProbeResult{}.TableName()+"_external (adapter_id varchar(512), probe_window_start timestamptz, FOREIGN KEY(adapter_id,probe_window_start) REFERENCES "+m.HealthProbeResult{}.TableName()+"(adapter_id,probe_window_start) ON DELETE CASCADE)").Error)
	_, e = r.Batch(ctx, repo.ProbeRetentionID, cutoff, now, 200, true)
	require.ErrorContains(t, e, "external_references")
	require.Error(t, r.Ready(ctx, "agent_chat_messages"))
	// 真实采样持久化 + Root HTTP 历史；只采元数据，不执行时间范围扫描。
	require.NoError(t, svc.CaptureSample(ctx))
	w = request(t, root, "GET", "/api/v1/admin/ops/storage/database-tables/"+repo.ProbeRetentionID+"/history", "", svc)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "allocated_bytes_net_change_not_ingress")
	w = request(t, root, "GET", "/api/v1/admin/ops/storage/database-tables/monitoring", "", svc)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"state":"disabled"`)
}
