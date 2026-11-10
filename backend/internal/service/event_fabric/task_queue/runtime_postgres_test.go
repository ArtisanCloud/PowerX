package taskqueue

import (
	"context"
	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	em "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	sm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"os"
	"testing"
	"time"
)

func runtimePostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("R1_CORE_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TASK_QUEUE_ISOLATED_POSTGRES_REQUIRED")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	require.NoError(t, err)
	sql, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sql.Close()) })
	var name string
	require.NoError(t, db.Raw("SELECT current_database()").Scan(&name).Error)
	require.Equal(t, "r1_core_fixture", name)
	return db
}
func TestRuntimeQueuePrepareIsolatedCatalog(t *testing.T) {
	db := runtimePostgres(t)
	require.NoError(t, db.AutoMigrate(&im.AuthorizationTracking{}, &im.AuthorizationStream{}, &im.AuthorizationSegment{}, &im.User{}, &im.Member{}, &im.Role{}, &im.Permission{}, &im.RolePermission{}, &im.RoleBinding{}, &im.MemberAssignment{}, &tm.Tenant{}, &sm.PluginInstanceConfig{}, &gm.IntegrationGatewayAPIKey{}, &gm.IntegrationGatewayAPIKeyPermission{}, &cm.CapabilityRecord{}, &cm.CapabilityRegistration{}, &cm.AdapterEndpoint{}, &cm.RoutingPolicy{}, &cm.FallbackPlan{}, &em.TaskHistory{}))
	tenant := tm.Tenant{Key: "r1_fixture", Name: "r1_fixture", Status: 1}
	tenant.UUID = uuid.MustParse("42424242-4242-4242-4242-424242424242")
	require.NoError(t, db.Create(&tenant).Error)
	// Nullable keys on native history permit schema migration without fabricated keys.
	for _, id := range []string{"fixture_native_a", "fixture_native_b"} {
		require.NoError(t, db.Create(&em.TaskHistory{TaskID: id, TenantKey: tenant.UUID.String(), SubscriberID: "fixture_native"}).Error)
	}
	require.NoError(t, db.AutoMigrate(&em.TaskHistory{}))
}
func TestRuntimeQueueSeededAPIKeyGrantStatus(t *testing.T) {
	db := runtimePostgres(t)
	tenant := "42424242-4242-4242-4242-424242424242"
	key := gm.IntegrationGatewayAPIKey{TenantUUID: tenant, ProfileID: 1, Name: "r1_fixture", KeyPrefix: "r1_fixture", KeyHash: uuid.NewString(), Status: "active"}
	require.NoError(t, db.Create(&key).Error)
	for _, v := range []struct{ scope, action, resource string }{{"_scope.runtime.taskqueue.enqueue", "enqueue", "runtime_taskqueue_enqueue"}, {"_scope.runtime.taskqueue.consume", "consume", "runtime_taskqueue_consume"}, {"_scope.capabilities.grant_status.read", "read", "grant-status"}} {
		kind := "capability"
		if v.resource == "grant-status" {
			kind = "api"
		}
		require.NoError(t, db.Create(&gm.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: v.scope, Action: v.action, ResourceType: kind, ResourcePattern: v.resource, Effect: "allow"}).Error)
	}
	ctx := reqctx.WithAuthenticatedAPIKeyHash(reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant), &reqctx.CoreXClaims{TenantUUID: tenant, MemberID: 1, Platforms: []string{"api_key"}}), key.KeyHash)
	status, err := cap.NewGrantStatusService(db).CheckCurrentCredential(ctx, []string{EnqueueCapability, ConsumeCapability})
	require.NoError(t, err)
	require.Len(t, status, 2)
	for _, v := range status {
		require.Equal(t, cap.GrantStatusGranted, v.Status)
		t.Log(v.CapabilityID, v.Status)
	}
	var rows []im.Permission
	require.NoError(t, db.Where("module = ? AND resource IN ?", "runtime_taskqueue", []string{"service_enqueue", "service_consume"}).Find(&rows).Error)
	require.Len(t, rows, 2)
	for _, row := range rows {
		require.True(t, row.AllowAPIKey)
	}
}

func TestRuntimeQueueRealRedisPostgresSettlement(t *testing.T) {
	url := os.Getenv("TASKQUEUE_TEST_REDIS_URL")
	if url == "" {
		t.Skip("TASK_QUEUE_ISOLATED_REDIS_REQUIRED")
	}
	db := runtimePostgres(t)
	options, err := redis.ParseURL(url)
	require.NoError(t, err)
	client := redis.NewClient(options)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	base, err := event_bus.NewRedisStreamTaskDriver(client, time.Second, "r1_fixture_"+uuid.NewString())
	require.NoError(t, err)
	service := NewRuntimeService(NewStreamDriver(db, base))
	tenant := uuid.NewString()
	scope := RuntimeScope{TenantUUID: tenant, SourceService: "fixture_service"}
	ctx := context.Background()
	msg := runtimeFixtureMessage()
	msg.TenantKey = tenant
	enq := RuntimeRequest{ContractVersion: RuntimeContract, Operation: "enqueue", TenantKey: tenant, Message: msg}
	for n := 0; n < 2; n++ {
		_, err := service.Apply(ctx, scope, enq)
		require.NoError(t, err)
	}
	take := RuntimeRequest{ContractVersion: RuntimeContract, Operation: "dequeue", TenantKey: tenant, SubscriberID: msg.SubscriberID, WorkerID: uuid.NewString(), MaxItems: 1, WaitTimeoutMS: 1}
	first, err := service.Apply(ctx, scope, take)
	require.NoError(t, err)
	items := first["messages"].([]RuntimeMessage)
	require.Len(t, items, 1)
	settle := RuntimeRequest{ContractVersion: RuntimeContract, Operation: "renew", TenantKey: tenant, SubscriberID: msg.SubscriberID, MessageID: msg.ID, ReceiptUUID: items[0].Lease.ReceiptUUID}
	_, err = service.Apply(ctx, scope, settle)
	require.NoError(t, err)
	at := time.Now().UTC().Add(40 * time.Millisecond)
	settle.Operation = "nack"
	settle.Reason = "FIXTURE_RETRY"
	settle.RetryAt = &at
	_, err = service.Apply(ctx, scope, settle)
	require.NoError(t, err)
	time.Sleep(60 * time.Millisecond)
	second, err := service.Apply(ctx, scope, take)
	require.NoError(t, err)
	next := second["messages"].([]RuntimeMessage)
	require.Len(t, next, 1)
	require.Greater(t, next[0].Lease.Fence, items[0].Lease.Fence)
	settle.Operation = "ack"
	settle.Reason = ""
	settle.RetryAt = nil
	settle.ReceiptUUID = next[0].Lease.ReceiptUUID
	settle.Outcome = "completed"
	_, err = service.Apply(ctx, scope, settle)
	require.NoError(t, err)
	var rows []em.TaskHistory
	require.NoError(t, db.Where("tenant_key = ? AND source = ?", tenant, "runtime_host_v2").Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, em.TaskHistoryStatusCompleted, rows[0].Status)
}
