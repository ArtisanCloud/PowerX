package integration_gateway

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	discoverycache "github.com/ArtisanCloud/PowerX/internal/infra/cache/discovery"
	discovery "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/discovery"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	audit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	gw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	r "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	tr "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newCatalogUpgradeDB(t *testing.T) *gorm.DB {
	t.Helper()
	before := core.PowerXSchema
	core.PowerXSchema = "main"
	t.Cleanup(func() { core.PowerXSchema = before })
	dir, err := filepath.Abs("../../../config/platform_capabilities")
	require.NoError(t, err)
	t.Setenv("PLATFORM_CAPABILITIES_DIR", dir)
	var db *gorm.DB
	if dsn := os.Getenv("POWERX_CATALOG_UPGRADE_TEST_DSN"); dsn != "" {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true})
		require.NoError(t, err)
		var name string
		require.NoError(t, db.Raw("select current_database()").Scan(&name).Error)
		require.True(t, strings.HasPrefix(name, "powerx_catalog_upgrade_test_"))
		core.PowerXSchema = "catalog_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		schema := core.PowerXSchema
		require.NoError(t, db.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
		pool, err := db.DB()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, db.Exec(`DROP SCHEMA "`+schema+`" CASCADE`).Error); _ = pool.Close() })
	} else {
		db, err = gorm.Open(sqlite.Open(t.TempDir()+"/catalog.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true})
		require.NoError(t, err)
	}
	pool, err := db.DB()
	require.NoError(t, err)
	if db.Dialector.Name() == "sqlite" {
		t.Cleanup(func() { _ = pool.Close() })
	}
	models := []any{&m.CapabilityRecord{}, &m.CapabilityRegistration{}, &m.AdapterEndpoint{}, &m.RoutingPolicy{}, &m.FallbackPlan{}, &audit.AuditEvent{}, &tm.Tenant{}, &gw.IntegrationGatewayAPIKey{}, &gw.IntegrationGatewayAPIKeyPermission{}}
	if db.Dialector.Name() == "sqlite" {
		pool.SetMaxOpenConns(1)
		for _, model := range models {
			stmt := &gorm.Statement{DB: db}
			require.NoError(t, stmt.Parse(model))
			for _, field := range stmt.Schema.Fields {
				if strings.Contains(field.DefaultValue, "(") {
					field.HasDefaultValue = false
					field.DefaultValue = ""
				}
			}
		}
	}
	require.NoError(t, db.AutoMigrate(models...))
	return db
}

func mediaDefinition(t *testing.T, id string) platformCapabilityDefinition {
	t.Helper()
	for _, def := range platformCapabilityDefinitions() {
		if def.CapabilityID == id {
			return def
		}
	}
	t.Fatalf("missing formal definition %s", id)
	return platformCapabilityDefinition{}
}

func catalogSeeder(db *gorm.DB) *BaseCapabilitySeeder {
	return NewBaseCapabilitySeeder(BaseCapabilitySeederOptions{RecordRepo: r.NewCapabilityRecordRepository(db, nil), RegistryRepo: r.NewCapabilityRegistryRepository(db), TenantRepo: tr.NewTenantRepository(db), PreserveInstalledState: true})
}

func seedLegacyMediaRegistration(t *testing.T, db *gorm.DB, tenant, id string) r.Registration {
	t.Helper()
	def := mediaDefinition(t, id)
	oldDef := def
	oldDef.Policy.Prefer = "rest"
	oldDef.Docs = []string{"specs/007-integration-gateway-and-mcp/contracts/ai-multimodal.http-openapi.yaml"}
	if strings.Contains(id, ".catalog.") {
		oldDef.Docs = []string{"backend/internal/transport/http/admin/agent/api.go"}
	}
	for _, protocol := range def.Protocols {
		if protocol.Channel == "rest" {
			oldDef.Protocols = []m.ProtocolBinding{protocol}
			break
		}
	}
	_, err := r.NewCapabilityRecordRepository(db, nil).Upsert(context.Background(), oldDef.toRecord(PlatformPluginID, PlatformPluginVersion, time.Now().Add(-24*time.Hour)))
	require.NoError(t, err)
	s := catalogSeeder(db)
	reg := s.buildPlatformRegistration(def, tenant, time.Now().Add(-24*time.Hour))
	reg.ContractRef = oldDef.Docs[0]
	// 明确写入旧 HTTP adapter，不能依赖已升级的生成器创建测试前置条件。
	binding := oldDef.Protocols[0]
	reg.Adapters = []r.AdapterEndpoint{{AdapterID: "legacy-" + id, TransportType: "http", Endpoint: binding.Endpoint, ServiceRef: binding.Endpoint, IsActive: true, Weight: 80, TimeoutMS: 7500, Labels: map[string]string{"method": binding.Method, "auth_type": "api_key_or_bearer"}}}
	reg.RoutingPolicy.FallbackSequence = []string{reg.Adapters[0].AdapterID}
	reg.EnvironmentPolicies["maintenance"] = r.EnvironmentPolicy{IsEnabled: false}
	reg.ToolGrantIDs = []string{"retained-grant"}
	got, err := r.NewCapabilityRegistryRepository(db).Create(context.Background(), nil, reg)
	require.NoError(t, err)
	return got
}

func TestCatalogUpgradeLegacyRegistrationsAllTenants(t *testing.T) {
	db := newCatalogUpgradeDB(t)
	ctx := context.Background()
	repository := r.NewCapabilityRegistryRepository(db)
	tenants := []string{uuid.NewString(), uuid.NewString()}
	for i, tenant := range tenants {
		row := tm.Tenant{Key: "catalog-" + tenant}
		row.UUID = uuid.MustParse(tenant)
		require.NoError(t, db.Create(&row).Error)
		if i == 1 {
			require.NoError(t, db.Model(&row).Update("status", tm.TenantStatusDisabled).Error)
		}
	}
	key := gw.IntegrationGatewayAPIKey{TenantUUID: tenants[0], ProfileID: 1, Name: "original-plugin-key", KeyPrefix: "fixture", KeyHash: "retained-hash", Status: "active"}
	require.NoError(t, db.Create(&key).Error)
	grant := gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: "_scope.ai.catalog.models.read", Action: "read", ResourceType: "api", ResourcePattern: "ai-catalog-models", Effect: "allow"}
	require.NoError(t, db.Create(&grant).Error)
	// 比较持久化前后的记录，避免 SQLite 将 time.Local 还原为相同偏移的固定时区造成误报。
	require.NoError(t, db.First(&key, key.ID).Error)
	require.NoError(t, db.First(&grant, grant.ID).Error)
	ids := []string{"com.corex.ai.catalog.providers.read", "com.corex.ai.catalog.models.read", "com.corex.ai.vlm.invoke", "com.corex.ai.image.invoke", "com.corex.ai.video.invoke", "com.corex.ai.tts.invoke", "com.corex.ai.media.tasks.read", "com.corex.ai.media.tasks.cancel"}
	for _, tenant := range tenants {
		for _, id := range ids {
			seedLegacyMediaRegistration(t, db, tenant, id)
		}
	}
	disabled, err := repository.GetLatest(ctx, nil, ids[0], tenants[1])
	require.NoError(t, err)
	disabled.Status, disabled.DisableReason = "disabled", "operator disabled"
	disabled.Adapters[0].IsActive = false
	previousVersion := disabled.Version
	disabled.Version++
	disabled.RoutingPolicy.ID = uuid.Nil
	_, err = repository.Update(ctx, nil, disabled, previousVersion)
	require.NoError(t, err)
	cacheStore := discoverycache.NewStore(cache.NewMemoryCache(), "catalog-test")
	discover := discovery.NewService(discovery.ServiceOptions{DB: db, CacheStore: cacheStore, DefaultTTL: time.Hour})
	before, err := discover.GetSnapshot(ctx, tenants[0], ids[1], "original-plugin")
	require.NoError(t, err)
	require.Equal(t, "http", before.Adapters[0].TransportType)
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cachedRepo := r.NewCapabilityRecordRepository(db, client)
	oldRecord, err := cachedRepo.GetByCapabilityID(ctx, ids[1])
	require.NoError(t, err)
	require.NoError(t, catalogSeeder(db).Ensure(ctx))
	newRecord, err := cachedRepo.GetByCapabilityID(ctx, ids[1])
	require.NoError(t, err)
	require.NotEqual(t, oldRecord.ProtocolHash, newRecord.ProtocolHash, "offline migration must fence the live record cache")
	for _, tenant := range tenants {
		for _, id := range ids {
			got, err := repository.GetLatest(ctx, nil, id, tenant)
			require.NoError(t, err)
			require.Greater(t, got.Version, uint64(1))
			require.Equal(t, "docs/contracts/ai-media-v2.openapi.yaml", got.ContractRef)
			require.Len(t, got.Adapters, 1)
			require.Equal(t, "core_internal", got.Adapters[0].TransportType)
			require.Equal(t, mediaRegistrationEndpoint(id), got.Adapters[0].Endpoint)
			require.Equal(t, got.Adapters[0].Endpoint, got.Adapters[0].ServiceRef)
			require.Equal(t, "INVOKE", got.Adapters[0].Labels["method"])
			require.Equal(t, []string{"retained-grant"}, got.ToolGrantIDs)
			require.False(t, got.EnvironmentPolicies["maintenance"].IsEnabled)
			require.Equal(t, 7500, got.Adapters[0].TimeoutMS)
			if tenant == tenants[1] && id == ids[0] {
				require.Equal(t, "disabled", got.Status)
				require.Equal(t, "operator disabled", got.DisableReason)
				require.False(t, got.Adapters[0].IsActive)
			}
			old, err := repository.GetVersion(ctx, nil, id, tenant, 1)
			require.NoError(t, err)
			require.Equal(t, "http", old.Adapters[0].TransportType)
		}
	}
	after, err := discover.GetSnapshot(ctx, tenants[0], ids[1], "original-plugin")
	require.NoError(t, err)
	require.Greater(t, after.Version, before.Version)
	require.Equal(t, "core_internal", after.Adapters[0].TransportType)
	var count int64
	require.NoError(t, db.Model(&audit.AuditEvent{}).Where("operation = ?", "UPGRADE_MEDIA_REGISTRATION").Count(&count).Error)
	require.EqualValues(t, 16, count)
	require.NoError(t, catalogSeeder(db).Ensure(ctx))
	var twice int64
	require.NoError(t, db.Model(&audit.AuditEvent{}).Where("operation = ?", "UPGRADE_MEDIA_REGISTRATION").Count(&twice).Error)
	require.Equal(t, count, twice)
	var retained gw.IntegrationGatewayAPIKey
	require.NoError(t, db.First(&retained, key.ID).Error)
	require.Equal(t, key, retained)
	var retainedGrant gw.IntegrationGatewayAPIKeyPermission
	require.NoError(t, db.First(&retainedGrant, grant.ID).Error)
	require.Equal(t, grant, retainedGrant)
}

func TestCatalogUpgradeConflictAndAuditFailureRollback(t *testing.T) {
	for _, mode := range []string{"custom_endpoint", "custom_contract", "audit_failure", "foreign_owner"} {
		t.Run(mode, func(t *testing.T) {
			db := newCatalogUpgradeDB(t)
			tenant := uuid.NewString()
			id := "com.corex.ai.catalog.models.read"
			reg := seedLegacyMediaRegistration(t, db, tenant, id)
			switch mode {
			case "custom_endpoint":
				require.NoError(t, db.Model(&m.AdapterEndpoint{}).Where("capability_id = ?", id).Update("endpoint", "https://custom.invalid/catalog").Error)
			case "custom_contract":
				require.NoError(t, db.Model(&m.CapabilityRegistration{}).Where("capability_id = ?", id).Update("contract_ref", "custom-contract").Error)
			case "audit_failure":
				require.NoError(t, db.Migrator().DropTable(&audit.AuditEvent{}))
			case "foreign_owner":
				require.NoError(t, db.Model(&m.CapabilityRecord{}).Where("capability_id = ?", id).Update("plugin_id", "custom-owner").Error)
			}
			var original m.CapabilityRecord
			require.NoError(t, db.Where("capability_id = ?", id).First(&original).Error)
			err := catalogSeeder(db).Ensure(context.Background())
			require.Error(t, err)
			got, err := r.NewCapabilityRegistryRepository(db).GetLatest(context.Background(), nil, id, tenant)
			require.NoError(t, err)
			require.Equal(t, reg.Version, got.Version)
			var current m.CapabilityRecord
			require.NoError(t, db.Where("capability_id = ?", id).First(&current).Error)
			require.Equal(t, original.ProtocolHash, current.ProtocolHash)
		})
	}
}

func TestCatalogUpgradeConcurrentVersions(t *testing.T) {
	db := newCatalogUpgradeDB(t)
	if db.Dialector.Name() != "postgres" {
		t.Skip("PostgreSQL advisory lock and real transaction concurrency")
	}
	tenant, id := uuid.NewString(), "com.corex.ai.catalog.models.read"
	old := seedLegacyMediaRegistration(t, db, tenant, id)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for n := 0; n < 2; n++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- catalogSeeder(db).Ensure(context.Background()) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	repository := r.NewCapabilityRegistryRepository(db)
	current, err := repository.GetLatest(context.Background(), nil, id, tenant)
	require.NoError(t, err)
	require.Equal(t, old.Version+1, current.Version)
	// 另一个管理员基于旧版本的更新必须冲突，不能追加第二个相同版本。
	_, err = repository.Update(context.Background(), nil, old, old.Version)
	require.ErrorIs(t, err, r.ErrVersionConflict)
	var count int64
	require.NoError(t, db.Model(&m.CapabilityRegistration{}).Where("tenant_uuid = ? AND capability_id = ?", tenant, id).Count(&count).Error)
	require.EqualValues(t, 2, count)
}

func TestCatalogUpgradeScopedSelectionAndUnknownID(t *testing.T) {
	db := newCatalogUpgradeDB(t)
	tenant, id := uuid.NewString(), "com.corex.ai.catalog.models.read"
	seedLegacyMediaRegistration(t, db, tenant, id)
	other := m.CapabilityRecord{CapabilityID: "com.corex.notifications.create", PluginID: "operator-owned", PluginVersion: "retained", Title: "retained", CapabilitiesHash: "retained", ProtocolHash: "retained", Status: "disabled"}
	require.NoError(t, db.Create(&other).Error)
	require.NoError(t, db.First(&other, other.ID).Error)
	s := catalogSeeder(db)
	s.capabilityIDs = []string{id}
	require.NoError(t, s.Ensure(context.Background()))
	var got m.CapabilityRecord
	require.NoError(t, db.Where("id = ?", other.ID).First(&got).Error)
	require.Equal(t, other, got)
	s.capabilityIDs = []string{"com.corex.unknown"}
	require.ErrorContains(t, s.Ensure(context.Background()), "UPGRADE_CAPABILITY_ID_UNKNOWN")
}
