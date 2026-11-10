package runtime_host

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	fixture "github.com/ArtisanCloud/PowerX/internal/testutil/runtime_host"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const cacheTenant = "00000000-0000-4000-8000-000000000001"

type cacheFixtureState struct {
	db       *gorm.DB
	redis    *redis.Client
	invoker  *CacheInvoker
	sts, api context.Context
	key      gm.IntegrationGatewayAPIKey
}

func cacheFixture(t *testing.T) *cacheFixtureState {
	t.Helper()
	db := fixture.Database(t)
	sts, credential := fixture.Actor(t, db, cacheTenant, "fixture-cache-plugin")
	for _, id := range []string{CacheServiceRead, CacheServiceManage} {
		require.NoError(t, db.Create(&cm.CapabilityRecord{CapabilityID: id, PluginID: "core", PluginVersion: "1", Title: id, CapabilitiesHash: id, ProtocolHash: id, Status: "published"}).Error)
		require.NoError(t, db.Create(&cm.CapabilityRegistration{CapabilityID: id, TenantUUID: cacheTenant, ContractRef: CacheServiceContract, Status: "published", Version: 1, RoutingPolicyID: uuid.New()}).Error)
	}
	fixture.Grant(t, db, credential, append(fixture.Capabilities, CacheServiceRead, CacheServiceManage)...)
	require.NoError(t, db.AutoMigrate(&im.Permission{}, &gm.IntegrationGatewayAPIKey{}, &gm.IntegrationGatewayAPIKeyPermission{}))
	key := gm.IntegrationGatewayAPIKey{TenantUUID: cacheTenant, ProfileID: 1, Name: "fixture-cache", KeyPrefix: "fixture-cache", KeyHash: uuid.NewString(), Status: "active"}
	require.NoError(t, db.Create(&key).Error)
	for _, action := range []string{"read", "manage"} {
		id := "com.corex.runtime.cache.service_" + action
		meta, _ := json.Marshal(map[string]any{"capability_id": id, "api_key_explicit": true, "api_key": map[string]any{"scope": "_scope.runtime.cache." + action, "action": action, "resource_type": "capability", "resource_pattern": "runtime_cache_" + action, "effect": "allow"}})
		require.NoError(t, db.Create(&im.Permission{Module: "runtime_cache", Resource: "service_" + action, Action: action, Effect: "allow", Status: im.PermissionStatusActive, AllowAPIKey: true, Meta: datatypes.JSON(meta)}).Error)
		require.NoError(t, db.Create(&gm.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: "_scope.runtime.cache." + action, Action: action, ResourceType: "capability", ResourcePattern: "runtime_cache_" + action, Effect: "allow"}).Error)
	}
	api := reqctx.WithAuthenticatedAPIKeyHash(reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), cacheTenant), &reqctx.CoreXClaims{TenantUUID: cacheTenant, MemberID: 1, Platforms: []string{"api_key"}}), key.KeyHash)
	var client *redis.Client
	if url := os.Getenv("X05_OWNED_REDIS_URL"); url != "" {
		opts, e := redis.ParseURL(url)
		require.NoError(t, e)
		client = redis.NewClient(opts)
	} else {
		server := miniredis.RunT(t)
		client = redis.NewClient(&redis.Options{Addr: server.Addr()})
	}
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	svc := NewService(db, client, "fixture-cache-environment")
	return &cacheFixtureState{db, client, NewCacheInvoker(db, svc), sts, api, key}
}
func cacheInput(operation string) cap.CoreCapabilityInvokeInput {
	op := map[string]any{"contract_version": CacheServiceContract, "tenant_key": cacheTenant, "operation": operation, "namespace": "fixture", "key": "fixture-key"}
	id := CacheServiceRead
	if operation == "set" {
		id = CacheServiceManage
		op["value_base64"] = base64.StdEncoding.EncodeToString([]byte("fixture-value"))
		op["ttl_ms"] = int64(60000)
	}
	if operation == "delete" {
		id = CacheServiceManage
	}
	return cap.CoreCapabilityInvokeInput{CapabilityID: id, TenantUUID: cacheTenant, Method: "INVOKE", Endpoint: CacheServiceEndpoint, Body: op, Payload: map[string]any{"body": op}}
}
func TestCacheTypedServiceCredentialsIsolationAndRevocation(t *testing.T) {
	f := cacheFixture(t)
	for _, ctx := range []context.Context{f.api, f.sts} {
		_, e := f.invoker.InvokeCoreCapability(ctx, cacheInput("set"))
		require.NoError(t, e)
		out, e := f.invoker.InvokeCoreCapability(ctx, cacheInput("get"))
		require.NoError(t, e)
		require.Equal(t, true, out["found"])
		require.Equal(t, base64.StdEncoding.EncodeToString([]byte("fixture-value")), out["value_base64"])
		_, e = f.invoker.InvokeCoreCapability(ctx, cacheInput("delete"))
		require.NoError(t, e)
		out, e = f.invoker.InvokeCoreCapability(ctx, cacheInput("get"))
		require.NoError(t, e)
		require.Equal(t, false, out["found"])
	}
	_, e := f.invoker.InvokeCoreCapability(f.api, cacheInput("set"))
	require.NoError(t, e)
	out, e := f.invoker.InvokeCoreCapability(f.sts, cacheInput("get"))
	require.NoError(t, e)
	require.Equal(t, false, out["found"])
	other := NewCacheInvoker(f.db, NewService(f.db, f.redis, "fixture-other-environment"))
	out, e = other.InvokeCoreCapability(f.api, cacheInput("get"))
	require.NoError(t, e)
	require.Equal(t, false, out["found"])
	require.NoError(t, f.db.Model(&f.key).Update("status", "revoked").Error)
	_, e = f.invoker.InvokeCoreCapability(f.api, cacheInput("get"))
	require.Equal(t, 401, dto.StatusCode(e))
	require.NoError(t, f.db.Create(&cm.CapabilityRegistration{CapabilityID: CacheServiceRead, TenantUUID: cacheTenant, ContractRef: CacheServiceContract, Status: "disabled", Version: 2, RoutingPolicyID: uuid.New()}).Error)
	_, e = f.invoker.InvokeCoreCapability(f.sts, cacheInput("get"))
	require.Equal(t, 403, dto.StatusCode(e))
}
func TestCacheTypedServiceRejectsProxyAndScopeOverride(t *testing.T) {
	f := cacheFixture(t)
	for _, mutate := range []func(*cap.CoreCapabilityInvokeInput){
		func(in *cap.CoreCapabilityInvokeInput) { in.Endpoint = "http://fixture.invalid" },
		func(in *cap.CoreCapabilityInvokeInput) { in.Method = "GET" },
		func(in *cap.CoreCapabilityInvokeInput) { in.Payload["headers"] = map[string]any{} },
		func(in *cap.CoreCapabilityInvokeInput) { in.Body["tenant_uuid"] = uuid.NewString() },
		func(in *cap.CoreCapabilityInvokeInput) { in.Body["contract_version"] = "old" },
		func(in *cap.CoreCapabilityInvokeInput) { in.Body["value_base64"] = "bad" },
	} {
		in := cacheInput("get")
		mutate(&in)
		_, e := f.invoker.InvokeCoreCapability(f.api, in)
		require.Equal(t, 400, dto.StatusCode(e))
	}
	for _, mutate := range []func(*cap.CoreCapabilityInvokeInput){func(in *cap.CoreCapabilityInvokeInput) { in.TenantUUID = uuid.NewString() }, func(in *cap.CoreCapabilityInvokeInput) { in.Body["tenant_key"] = uuid.NewString() }, func(in *cap.CoreCapabilityInvokeInput) { in.CapabilityID = CacheServiceManage }} {
		in := cacheInput("get")
		mutate(&in)
		_, e := f.invoker.InvokeCoreCapability(f.api, in)
		require.Equal(t, 403, dto.StatusCode(e))
	}
	in := cacheInput("set")
	in.Body["ttl_ms"] = int64(86400001)
	_, e := f.invoker.InvokeCoreCapability(f.sts, in)
	require.Equal(t, 400, dto.StatusCode(e))
	_, e = f.invoker.InvokeCoreCapability(reqctx.WithTenantUUID(context.Background(), cacheTenant), cacheInput("get"))
	require.Equal(t, 401, dto.StatusCode(e))
}

// This owned fixture connects the actual Framework client in the plugin test
// process to this Core invoker/store. Authentication is explicitly controlled;
// publication, registrations and live service grants use real database records.
func TestCacheFrameworkBridgeFixture(t *testing.T) {
	file := os.Getenv("X05_CORE_BRIDGE_FILE")
	if file == "" {
		t.Skip("X05_OWNED_CORE_BRIDGE_REQUIRED")
	}
	f := cacheFixture(t)
	done := make(chan struct{}, 1)
	// The official seed command runs against an owned public-schema copy. The
	// handoff below then opens a fresh GORM schema cache against that seeded DB.
	f.db.Config.DisableForeignKeyConstraintWhenMigrating = true
	require.NoError(t, f.db.AutoMigrate(&cm.AdapterEndpoint{}, &cm.RoutingPolicy{}, &cm.FallbackPlan{}))
	metadata, _ := json.Marshal(map[string]string{"schema": coremodel.PowerXSchema})
	require.NoError(t, os.WriteFile(file+".meta", metadata, 0o600))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fixture/use-seeded" {
			coremodel.PowerXSchema = "public"
			db, e := gorm.Open(postgres.Open(os.Getenv("POWERX_RUNTIME_HOST_TEST_POSTGRES_DSN")), &gorm.Config{})
			require.NoError(t, e)
			sql, e := db.DB()
			require.NoError(t, e)
			t.Cleanup(func() { require.NoError(t, sql.Close()) })
			f.db = db
			f.invoker = NewCacheInvoker(db, NewService(db, f.redis, "fixture-cache-environment"))
			w.WriteHeader(200)
			return
		}
		if r.URL.Path == "/fixture/stop" {
			done <- struct{}{}
			w.WriteHeader(200)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		var in struct {
			Capability string         `json:"capability_id"`
			Payload    map[string]any `json:"payload"`
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			w.WriteHeader(400)
			return
		}
		ctx := f.api
		switch r.Header.Get("Authorization") {
		case "ApiKey fixture-key":
		case "Bearer fixture-sts":
			ctx = f.sts
		default:
			w.WriteHeader(401)
			return
		}
		out, e := f.invoker.InvokeCoreCapability(ctx, cap.CoreCapabilityInvokeInput{CapabilityID: in.Capability, TenantUUID: cacheTenant, Method: "INVOKE", Endpoint: CacheServiceEndpoint, Body: in.Payload["body"].(map[string]any), Payload: in.Payload})
		if e != nil {
			w.WriteHeader(dto.StatusCode(e))
			_ = json.NewEncoder(w).Encode(map[string]any{"error_code": e.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"status": "completed", "protocol_used": "core_internal", "fallback_used": false, "payload": out}})
	}))
	t.Cleanup(server.Close)
	t.Logf("owned_fixture_listener=%s pid=%d", server.Listener.Addr(), os.Getpid())
	require.NoError(t, os.WriteFile(file, []byte(server.URL), 0o600))
	select {
	case <-done:
	case <-time.After(120 * time.Second):
		t.Fatal("CACHE_BRIDGE_TIMEOUT")
	}
}
