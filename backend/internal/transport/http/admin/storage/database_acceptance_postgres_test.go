package storage

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gatewayv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/integration_gateway/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	"github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	gov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	capgrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/capability_registry"
	caphttp "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/capability_registry"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	tenants "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	caprepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// 只接受独立集群的显式标识；不加载日常配置，不允许连接默认 PG 端口。
func acceptanceDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	clusterID := os.Getenv("POWERX_STORAGE_TEST_CLUSTER_ID")
	if clusterID == "" {
		t.Skip("opt-in owned PostgreSQL acceptance cluster")
	}
	parsed, err := pgx.ParseConfig(os.Getenv("POWERX_STORAGE_TEST_DSN"))
	require.NoError(t, err)
	require.NotEqualValues(t, 5432, parsed.Port)
	conn, err := pgx.ConnectConfig(context.Background(), parsed)
	require.NoError(t, err)
	var actual string
	err = conn.QueryRow(context.Background(), "SELECT system_identifier::text FROM pg_control_system()").Scan(&actual)
	require.NoError(t, conn.Close(context.Background()))
	require.NoError(t, err)
	require.Equal(t, clusterID, actual, "refuse a different PostgreSQL cluster before any schema mutation")
	db := storageNativeDB(t)
	require.NoError(t, db.AutoMigrate(&m.InvocationTrace{}, &m.CapabilityEventPublication{}, &m.CapabilityRecord{}, &tenants.Tenant{}))
	return db
}

type acceptanceRouter struct {
	registry.Repository
	registration registry.Registration
	routes       int
}

func (r *acceptanceRouter) GetLatest(context.Context, *gorm.DB, string, string) (registry.Registration, error) {
	r.routes++
	return r.registration, nil
}

type acceptanceBusiness struct{ calls int }

func (b *acceptanceBusiness) InvokeCoreCapability(context.Context, capservice.CoreCapabilityInvokeInput) (map[string]interface{}, error) {
	b.calls++
	return map[string]interface{}{"accepted": true}, nil
}

// 组合真实 Admin HTTP、策略服务、PG 仓储、Selector、调用审计和 gRPC。
// 鉴权上下文、业务适配器及路由登记为受控 fixture，不冒充日常实例的登录验收。
func TestDatabaseRetentionNativeAPIAndAdmission(t *testing.T) {
	db := acceptanceDatabase(t)
	for name, expected := range map[string]string{"logging_collector": "on", "log_statement": "mod"} {
		var actual string
		require.NoError(t, db.Raw("SHOW "+name).Scan(&actual).Error)
		require.Equal(t, expected, actual, "SQL write logging must remain enabled during cleanup")
	}
	ctx := reqctx.WithIsRoot(reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true}), true)
	root := reqctx.CoreXClaims{UserID: 1, IsRoot: true}
	tenant := &tenants.Tenant{Key: "owned-retention-test", Name: "Owned retention test"}
	require.NoError(t, db.Create(tenant).Error)
	tenantID := tenant.UUID.String()
	path := "/api/v1/admin/ops/storage/database-retention"
	dir := t.TempDir()
	coordinator, err := budget.New(budget.Config{Directory: filepath.Join(dir, "runtime"), FilesystemPath: dir, HostID: "owned-host", FilesystemID: "owned-fs", InstanceID: "owned-test", MinFreeBytes: 1 << 20, CriticalFreeBytes: 1})
	require.NoError(t, err)
	svc, err := gov.NewService(gov.Options{DB: db, Collector: testCollector{}, FilesystemPath: dir, Budget: coordinator, Identity: gov.Identity{HostID: "owned-host", FilesystemID: "owned-fs", InstanceID: "owned-test"}, DatabaseRetentionAllowed: true, LifecycleWorkerEnabled: true})
	require.NoError(t, err)
	read := func() gov.DatabaseRetentionStatus {
		w := request(t, root, "GET", path, "", svc)
		require.Equal(t, 200, w.Code, w.Body.String())
		var out struct {
			Data gov.DatabaseRetentionStatus `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		return out.Data
	}
	save := func(p gov.DatabaseRetentionPolicy) gov.DatabaseRetentionPolicy {
		raw, e := json.Marshal(map[string]interface{}{"expected_revision": p.Revision, "confirm": true, "policy": p})
		require.NoError(t, e)
		w := request(t, root, "PUT", path, string(raw), svc)
		require.Equal(t, 200, w.Code, w.Body.String())
		return read().Policy
	}
	initial := read()
	require.True(t, initial.Ready)
	require.False(t, initial.Policy.Enabled)
	require.False(t, initial.Policy.RejectNewWhenFull)
	require.NoFileExists(t, filepath.Join(dir, "runtime", "database-retention.json"))

	const capability = "com.corex.owned.storage-acceptance"
	require.NoError(t, db.Create(&m.CapabilityRecord{CapabilityID: capability, PluginID: "corex.platform", PluginVersion: "test", Title: "Owned test", Status: "published"}).Error)
	routes := &acceptanceRouter{registration: registry.Registration{CapabilityID: capability, TenantUUID: tenantID, Version: 1, Status: "published", Adapters: []registry.AdapterEndpoint{{AdapterID: "owned-core", TransportType: "core_internal", Endpoint: "core://owned/storage-acceptance", IsActive: true, Labels: map[string]string{"method": "INVOKE"}}}}}
	business := &acceptanceBusiness{}
	audit := capservice.NewAuditService(capservice.AuditServiceOptions{TraceRepo: caprepo.NewInvocationTraceRepository(db), EventRepo: caprepo.NewCapabilityEventPublicationRepository(db)})
	audit.SetStorageAdmission(svc.CheckDatabaseAdmission)
	catalog := capservice.NewRegistryService(capservice.RegistryServiceOptions{DB: db})
	invoker := capservice.NewInvocationService(capservice.InvocationServiceOptions{Catalog: catalog, Router: router.NewService(router.ServiceOptions{RegistryRepository: routes}), Audit: audit, CoreInvoker: business})
	deps := &shared.Deps{DB: db, CapabilityCatalogSvc: catalog, CapabilityInvocationSvc: invoker}
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(reqctx.WithClaims(reqctx.WithTenantUUID(c.Request.Context(), tenantID), &reqctx.CoreXClaims{UserID: 1, TenantUUID: tenantID}))
		c.Next()
	})
	caphttp.RegisterTenantRoutes(engine.Group("/api/v1"), deps)
	invoke := func() *httptest.ResponseRecorder {
		body := fmt.Sprintf(`{"capability_id":%q,"preferred_protocol":"core_internal","payload":{"operation":"test"}}`, capability)
		r := httptest.NewRequest("POST", "/api/v1/tenant/invocations", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		return w
	}
	counts := func() [2]int64 {
		var out [2]int64
		require.NoError(t, db.Unscoped().Model(&m.InvocationTrace{}).Count(&out[0]).Error)
		require.NoError(t, db.Unscoped().Model(&m.CapabilityEventPublication{}).Count(&out[1]).Error)
		return out
	}
	w := invoke()
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, 1, business.calls)
	require.Equal(t, [2]int64{1, 1}, counts(), "successful execution must still record trace and publication")

	p := initial.Policy
	p.RejectNewWhenFull = true // 删除保持关闭，容量保护独立开启。
	p.MaxBytes = 64 << 20
	p = save(p)
	w = invoke()
	require.Equal(t, 200, w.Code, w.Body.String())
	before := counts()
	budgetPolicy, err := coordinator.Policy(ctx)
	require.NoError(t, err)
	budgetPolicy.MaxUsedBytes = 1 // 实测整盘占用必然超过 1 byte，无需填满磁盘。
	budgetPolicy, err = coordinator.SavePolicy(ctx, budgetPolicy.Revision, budgetPolicy)
	require.NoError(t, err)
	w = invoke()
	require.Equal(t, 503, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "STORAGE_CAPACITY_EXHAUSTED")
	require.Equal(t, 2, business.calls)
	require.Equal(t, before, counts(), "rejected request must not execute or add an invocation audit row")
	_, err = capgrpc.NewIntegrationGatewayServer(deps).InvokeCapability(ctx, &gatewayv1.InvokeCapabilityRequest{CapabilityId: capability, TenantUuid: tenantID, PreferredProtocol: "core_internal"})
	require.Equal(t, codes.Unavailable, status.Code(err))
	require.Contains(t, err.Error(), "STORAGE_CAPACITY_EXHAUSTED")
	require.Equal(t, 2, routes.routes, "capacity rejection must precede router mutation")
	budgetPolicy.MaxUsedBytes = 0
	budgetPolicy, err = coordinator.SavePolicy(ctx, budgetPolicy.Revision, budgetPolicy)
	require.NoError(t, err)
	space, err := coordinator.Status(ctx)
	require.NoError(t, err)
	budgetPolicy.CriticalFreeBytes = space.AvailableBytes + (512 << 20)
	budgetPolicy.MinFreeBytes = budgetPolicy.CriticalFreeBytes + (1 << 20)
	budgetPolicy, err = coordinator.SavePolicy(ctx, budgetPolicy.Revision, budgetPolicy)
	require.NoError(t, err)
	w = invoke()
	require.Equal(t, 503, w.Code, "insufficient host reserve must block without filling the real disk")
	require.Equal(t, before, counts())
	budgetPolicy.MinFreeBytes, budgetPolicy.CriticalFreeBytes = 1<<20, 1
	budgetPolicy, err = coordinator.SavePolicy(ctx, budgetPolicy.Revision, budgetPolicy)
	require.NoError(t, err)
	lease, err := coordinator.Reserve(ctx, "owned-acceptance", 32<<20, time.Minute)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lease.Release(context.Background()) })
	space, err = coordinator.Status(ctx)
	require.NoError(t, err)
	budgetPolicy.MaxUsedBytes = space.UsedBytes + (16 << 20)
	budgetPolicy, err = coordinator.SavePolicy(ctx, budgetPolicy.Revision, budgetPolicy)
	require.NoError(t, err)
	w = invoke()
	require.Equal(t, 503, w.Code, "other jobs' reservations must count towards shared host capacity")
	require.Equal(t, before, counts())
	require.Equal(t, 2, business.calls)
	require.NoError(t, lease.Release(context.Background()))
	budgetPolicy.MaxUsedBytes = 0
	_, err = coordinator.SavePolicy(ctx, budgetPolicy.Revision, budgetPolicy)
	require.NoError(t, err)

	// 真实关系占用超过 64 MiB；TOAST 使用不可压缩数据，不依赖伪造采样。
	noise := make([]byte, 2<<20)
	_, err = rand.Read(noise)
	require.NoError(t, err)
	payload, err := json.Marshal(map[string]string{"owned_canary": base64.StdEncoding.EncodeToString(noise)})
	require.NoError(t, err)
	old := time.Now().UTC().AddDate(0, 0, -100)
	for i := 0; i < 28; i++ {
		require.NoError(t, db.Create(&m.InvocationTrace{TenantUUID: tenantID, TraceID: uuid.NewString(), CapabilityID: capability, PluginID: "corex.platform", Status: "completed", RecordClass: m.InvocationAuditClass, CompletedAt: &old, RequestPayload: datatypes.JSON(payload)}).Error)
	}
	full := read()
	require.NotNil(t, full.RelationBytes)
	require.GreaterOrEqual(t, *full.RelationBytes, p.MaxBytes)
	before = counts()
	w = invoke()
	require.Equal(t, 503, w.Code, w.Body.String())
	require.Equal(t, before, counts())
	require.Equal(t, 2, business.calls)
	require.False(t, read().Policy.Enabled)

	// 显式预览、启用、批次及重试；不把行删除宣称为文件系统立即释放。
	preview := request(t, root, "POST", path+"/previews", fmt.Sprintf(`{"expected_revision":%d}`, p.Revision), svc)
	require.Equal(t, 200, preview.Code, preview.Body.String())
	require.Contains(t, preview.Body.String(), `"traces":28`)
	require.Equal(t, before, counts())
	p.Enabled = true
	p = save(p)
	key := uuid.NewString()
	body := fmt.Sprintf(`{"expected_revision":%d,"request_id":%q,"confirm":true}`, p.Revision, key)
	w = request(t, root, "POST", path+"/runs", body, svc)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"traces":28`)
	result := w.Body.String()
	w = request(t, root, "POST", path+"/runs", body, svc)
	require.Equal(t, result, w.Body.String(), "same request ID must not replay deletion")
	require.Equal(t, [2]int64{2, 2}, counts(), "recent unconfirmed audit chains remain intact")
	after := read()
	require.NotNil(t, after.RelationBytes)
	require.Contains(t, after.PhysicalReclaim, "not_immediate_filesystem_release")
	t.Logf("relation bytes before=%d after_delete=%d (physical high-water budget remains authoritative)", *full.RelationBytes, *after.RelationBytes)
	if *after.RelationBytes >= p.MaxBytes {
		w = invoke()
		require.Equal(t, 503, w.Code, "deleting rows must not bypass a still-exhausted physical budget")
	}
	for _, table := range []string{m.InvocationTrace{}.TableName(), m.CapabilityEventPublication{}.TableName()} {
		require.NoError(t, db.Exec("VACUUM (ANALYZE) "+table).Error)
	}
	vacuumed := read()
	require.NotNil(t, vacuumed.RelationBytes)
	require.Equal(t, [2]int64{2, 2}, counts())
	t.Logf("ordinary VACUUM relation bytes=%d; no VACUUM FULL or file/WAL deletion", *vacuumed.RelationBytes)
	p.Enabled = false
	p = save(p)
	// 退出预算保护必须单独确认；暂停删除不能隐式撤掉保护。
	require.True(t, p.RejectNewWhenFull)
	p.RejectNewWhenFull = false
	p = save(p)
	w = invoke()
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, 3, business.calls)
	require.Equal(t, [2]int64{3, 3}, counts())

	// 真实 worker 启动周期按 2 行批次推进 5 行，包含最后一次空批；不调用手动 runs。
	for i := 0; i < 5; i++ {
		require.NoError(t, db.Create(&m.InvocationTrace{TenantUUID: tenantID, TraceID: uuid.NewString(), CapabilityID: capability, PluginID: "corex.platform", Status: "completed", RecordClass: m.InvocationAuditClass, CompletedAt: &old}).Error)
	}
	p.Enabled, p.BatchLimit = true, 2
	p = save(p)
	changes, unsubscribe := svc.SubscribeRuntime()
	defer unsubscribe()
	workerCtx, stopWorker := context.WithCancel(context.Background())
	defer stopWorker()
	workerErrors := make(chan error, 8)
	svc.StartRuntime(workerCtx, func(err error) { workerErrors <- err })
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for completed := 0; completed < 4; {
		select {
		case event := <-changes:
			if event.Reason == "database_retention_completed" {
				completed++
			}
		case err := <-workerErrors:
			t.Fatalf("native worker failed: %v", err)
		case <-deadline.C:
			t.Fatal("native worker did not complete its bounded batches")
		}
	}
	stopWorker()
	automatic := read()
	var scheduledDeletes int64
	for _, run := range automatic.Runs {
		if run.Revision == p.Revision {
			require.Equal(t, "completed", run.State)
			require.Zero(t, run.Actor, "automatic batches do not pretend to be manual Root requests")
			require.LessOrEqual(t, run.Counts.Traces, int64(2))
			scheduledDeletes += run.Counts.Traces
		}
	}
	require.EqualValues(t, 5, scheduledDeletes)
	require.Equal(t, [2]int64{3, 3}, counts())
	p.Enabled = false
	p = save(p)

	// 缺失任意目标关系时，容量未知必须失败关闭，不能把 SQL NULL 当 0 byte。
	require.NoError(t, db.Migrator().DropTable(&m.CapabilityEventPublication{}))
	_, err = caprepo.NewRetentionRepository(db).RelationBytes(ctx)
	require.Error(t, err)
}
