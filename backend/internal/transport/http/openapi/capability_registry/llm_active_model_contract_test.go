package capability_registry

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	stsv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/auth/sts/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	aimodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	airepo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	agentsettings "github.com/ArtisanCloud/PowerX/internal/service/agent"
	aisvc "github.com/ArtisanCloud/PowerX/internal/service/ai"
	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	caprouter "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	gateway "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway"
	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	settingsvc "github.com/ArtisanCloud/PowerX/internal/service/setting"
	authgrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/auth"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tenantmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	caprepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	tenantrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var llmSTSFixtureRedis *miniredis.Miniredis

func TestMain(m *testing.M) {
	code := m.Run()
	if llmSTSFixtureRedis != nil {
		llmSTSFixtureRedis.Close()
	}
	os.Exit(code)
}

type activeModelStreamFixture struct {
	*aisvc.Service
	env, model string
}

func (p *activeModelStreamFixture) LLMStream(_ context.Context, env, _, model string, _ []aisvc.ContentItem, _ map[string]any, delta func(string)) (string, error) {
	p.env, p.model = env, model
	delta("fixture")
	return "fixture", nil
}

func TestSTSReadsAndCallsTheSavedDefaultTextModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	t.Setenv("PLATFORM_CAPABILITIES_DIR", "../../../../../config/platform_capabilities")
	previousSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previousSchema })
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/active-model.db"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	models := []any{
		&aimodel.AIRoutePolicy{}, &aimodel.AIModelProfile{}, &setting.TenantSetting{}, &setting.PluginInstanceConfig{},
		&cm.CapabilityRecord{}, &cm.CapabilityRegistration{}, &cm.RoutingPolicy{}, &cm.AdapterEndpoint{}, &cm.FallbackPlan{},
		&tenantmodel.Tenant{}, &im.Permission{},
	}
	for _, model := range models {
		stmt := &gorm.Statement{DB: db}
		require.NoError(t, stmt.Parse(model))
		for _, field := range stmt.Schema.Fields {
			field.DefaultValue = strings.TrimSuffix(field.DefaultValue, "::jsonb")
		}
	}
	require.NoError(t, db.AutoMigrate(models...))

	tenant := tenantmodel.Tenant{Key: "starrtoy-fixture"}
	tenant.UUID = uuid.New()
	require.NoError(t, db.Create(&tenant).Error)
	tenantID := tenant.UUID.String()
	ids := []string{aisvc.LLMActiveModelCapability, aisvc.LLMStreamCapability, cap.GrantStatusCapabilityID}
	require.NoError(t, apikeypermissions.EnsureUpgradePermissionsForCapabilities(ctx, db, ids))
	registry := caprepo.NewCapabilityRegistryRepository(db)
	require.NoError(t, gateway.NewBaseCapabilitySeeder(gateway.BaseCapabilitySeederOptions{
		CapabilityIDs: ids, PreserveInstalledState: true, RecordRepo: caprepo.NewCapabilityRecordRepository(db, nil),
		RegistryRepo: registry, TenantRepo: tenantrepo.NewTenantRepository(db),
	}).Ensure(ctx))
	reg, err := registry.GetLatest(ctx, nil, aisvc.LLMActiveModelCapability, tenantID)
	require.NoError(t, err)
	require.Len(t, reg.Adapters, 1)
	require.Equal(t, aisvc.LLMActiveModelEndpoint, reg.Adapters[0].Endpoint)

	settings := agentsettings.NewAgentSettingService(db)
	require.NoError(t, settings.SetTenantCurrentAIEnv(ctx, tenantID, "staging"))
	require.NoError(t, settings.SetActiveProfile(ctx, "staging", &tenantID, "llm", "qwen-cn", "qwen3.8-flash"))
	require.NoError(t, airepo.NewAIRoutePolicyRepository(db).UpsertDefaultByScopeModality(ctx, "default", &tenantID, "llm", "ollama", "legacy:latest"))
	foreign := uuid.NewString()
	require.NoError(t, settings.SetActiveProfile(ctx, "prod", &foreign, "llm", "ollama", "foreign"))
	require.NoError(t, db.Create(&aimodel.AIModelProfile{Env: "staging", TenantUUID: &tenantID, Modality: "llm", Provider: "qwen-cn", Model: "qwen3.8-flash"}).Error)
	require.NoError(t, db.Create(&aimodel.AIModelProfile{Env: "prod", TenantUUID: &tenantID, Modality: "llm", Provider: "ollama", Model: "other-configured-model"}).Error)

	const pluginID = "com.example.starrtoy"
	clientID := pluginID + "." + tenantID
	const secret = "fixture-sts-client-secret"
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.MinCost)
	require.NoError(t, err)
	credential := setting.PluginInstanceConfig{TenantUUID: tenantID, PluginID: pluginID, Key: settingsvc.KeyClientCredentials, Enabled: true}
	setGrants := func(allowed []string) {
		raw, err := json.Marshal(settingsvc.ClientCredential{ClientID: clientID, ClientSecretHash: string(hash), AllowedCapabilities: allowed})
		require.NoError(t, err)
		credential.ValueJSON = raw
		require.NoError(t, db.Save(&credential).Error)
	}
	setGrants(ids[1:])
	if llmSTSFixtureRedis == nil {
		llmSTSFixtureRedis, err = miniredis.Run()
		require.NoError(t, err)
		port, err := strconv.Atoi(llmSTSFixtureRedis.Port())
		require.NoError(t, err)
		_, err = cache.InitCache(&cache.CacheConfig{Driver: "redis", Host: llmSTSFixtureRedis.Host(), Port: port})
		require.NoError(t, err)
	}
	signingKey := []byte("fixture-sts-signing-key")
	ring := authgrpc.NewKeyRing()
	ring.AddHS("default", signingKey)
	sts := authgrpc.NewSTSServiceServerWithRing(&shared.Deps{DB: db}, ring)
	issued, err := sts.Exchange(ctx, &stsv1.ExchangeRequest{ClientId: clientID, ClientSecret: secret})
	require.NoError(t, err)
	require.EqualValues(t, 200, issued.GetMeta().GetCode(), issued.GetMeta().GetMessage())
	require.NotEmpty(t, issued.GetData().GetAccessToken())

	ai := aisvc.NewService(db)
	provider := &activeModelStreamFixture{Service: ai}
	catalog := cap.NewRegistryService(cap.RegistryServiceOptions{DB: db})
	invocation := cap.NewInvocationService(cap.InvocationServiceOptions{
		Catalog: catalog, Router: caprouter.NewService(caprouter.ServiceOptions{DB: db}),
		ModelVerifier: cap.NewTenantModelKeyVerifier(db), TenantAIEnvironment: ai.ResolveTenantEnv,
		CoreInvoker: cap.NewCoreCapabilityMux(&aisvc.LLMActiveModelInvoker{Service: ai}, &aisvc.LLMStreamInvoker{Service: provider}),
	})
	engine := gin.New()
	engine.Use(middleware.APIKeyOrJwtMiddleware(db, signingKey, "powerx", []string{"powerx:user"}, []string{"access"}, nil,
		middleware.WithExtraTokenChecks(middleware.TokenCheck{Issuer: "powerx-sts", Audiences: []string{"powerx:api"}})))
	RegisterTenantRoutes(engine.Group("/api/v1"), &shared.Deps{DB: db, CapabilityCatalogSvc: catalog, CapabilityInvocationSvc: invocation,
		CapabilitySelector: cap.NewSelector(cap.SelectorOptions{Invoker: invocation}),
	})
	request := func(path string, payload map[string]any) *httptest.ResponseRecorder {
		raw, err := json.Marshal(payload)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+issued.GetData().GetAccessToken())
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w
	}
	const invokePath = "/api/v1/tenant/invocations"
	active := map[string]any{"capability_id": aisvc.LLMActiveModelCapability, "preferred_protocol": "core_internal",
		"payload": map[string]any{"body": map[string]any{"operation": "get_active_model"}}}
	w := request(invokePath, active)
	require.Equal(t, 403, w.Code, w.Body.String())
	setGrants(ids)
	w = request("/api/v1/tenant/capabilities:grant-status", map[string]any{"capability_ids": []string{aisvc.LLMActiveModelCapability}})
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "CAPABILITY_GRANTED")
	w = request(invokePath, active)
	require.Equal(t, 200, w.Code, w.Body.String())
	var read struct {
		Data struct {
			Result map[string]string `json:"result"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &read))
	require.Equal(t, map[string]string{"env": "staging", "modality": "llm", "model_key": "qwen-cn/qwen3.8-flash"}, read.Data.Result)
	stream := map[string]any{"capability_id": aisvc.LLMStreamCapability, "preferred_protocol": "core_internal",
		"payload": map[string]any{"body": map[string]any{"model_key": read.Data.Result["model_key"], "inputs": []any{map[string]string{"type": "text", "content": "fixture"}}}}}
	w = request(invokePath+"/stream", stream)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "event: done")
	require.Equal(t, "staging", provider.env)
	require.Equal(t, read.Data.Result["model_key"], provider.model)
	require.Equal(t, 403, request(invokePath+"/stream?env=prod", stream).Code)
	require.Equal(t, 400, request(invokePath+"?env=prod", active).Code)
	body := active["payload"].(map[string]any)["body"].(map[string]any)
	body["tenant_uuid"] = foreign
	require.Equal(t, 400, request(invokePath, active).Code)
	delete(body, "tenant_uuid")

	require.NoError(t, settings.SetTenantCurrentAIEnv(ctx, tenantID, "prod"))
	w = request(invokePath, active)
	require.Equal(t, 409, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "AI_DEFAULT_MODEL_NOT_CONFIGURED")
	require.NoError(t, settings.SetTenantCurrentAIEnv(ctx, tenantID, "dev"))
	w = request(invokePath, active)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"model_key":"ollama/legacy:latest"`)
	require.NoError(t, settings.SetActiveProfile(ctx, "dev", &tenantID, "llm", "qwen-cn", "qwen3.8-flash"))
	w = request(invokePath, active)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"model_key":"qwen-cn/qwen3.8-flash"`)
	require.NoError(t, db.Where("tenant_uuid = ? AND key = ?", tenantID, agentsettings.TenantSettingKeyAICurrentEnv).Delete(&setting.TenantSetting{}).Error)
	w = request(invokePath, active)
	require.Equal(t, 409, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "AI_ENV_NOT_CONFIGURED")
	setGrants(ids[1:])
	require.Equal(t, 403, request(invokePath, active).Code)
}
