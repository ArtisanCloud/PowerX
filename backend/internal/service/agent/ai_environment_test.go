package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	am "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func environmentAliasFixture(t *testing.T) *AgentSettingService {
	t.Helper()
	svc, _ := onlineCatalogFixture(t)
	for _, statement := range []string{
		`CREATE UNIQUE INDEX ai_cred_uniq_tenant ON ai_provider_credentials(env,tenant_uuid,name,provider)`,
		`CREATE UNIQUE INDEX ai_model_uniq_tenant ON ai_model_profiles(env,tenant_uuid,modality,provider,model)`,
		`CREATE TABLE ai_route_policies(id integer primary key,created_at datetime,updated_at datetime,deleted_at datetime,env text,tenant_uuid text,modality text,agent_id text,flow_id text,purpose text,provider text,model text,strategy text,compliance text,quota text)`,
	} {
		require.NoError(t, svc.db.Exec(statement).Error)
	}
	pool, err := svc.db.DB()
	require.NoError(t, err)
	connection, err := pool.Conn(context.Background())
	require.NoError(t, err)
	require.NoError(t, connection.Raw(func(driver any) error {
		return driver.(*sqlite3.SQLiteConn).RegisterFunc("NOW", func() string { return time.Now().UTC().Format(time.RFC3339Nano) }, false)
	}))
	require.NoError(t, connection.Close())
	return svc
}

func TestDevelopmentAliasReadsAndDecryptsLegacyProviderWithoutChangingStoredCredentials(t *testing.T) {
	s := environmentAliasFixture(t)
	tenant := onlineTestTenant
	ctx := context.Background()
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "Bearer fixture-key", r.Header.Get("Authorization"))
		require.Equal(t, "/v1/models", r.URL.Path)
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o"}]}`))
	}))
	defer provider.Close()
	savedOnlineCredential(t, s, tenant, "default", "openai", provider.URL+"/v1")
	// 复现现场：dev 只有系统级空 Key 记录，不能覆盖当前租户的旧凭据。
	require.NoError(t, s.db.Create(&am.AIProviderCredential{Env: "dev", Name: "dev-openai", Provider: "openai", Data: datatypes.JSONMap{"base_url": "https://system-placeholder.invalid/v1"}}).Error)
	legacy, err := s.credRepo.FindByScopeNameProvider(ctx, "default", &tenant, "default-openai", "openai")
	require.NoError(t, err)
	before, _ := json.Marshal(legacy.Data)
	for _, env := range []string{"dev", "default"} {
		credentials, err := s.ListCredentials(ctx, env, &tenant)
		require.NoError(t, err)
		require.Len(t, credentials, 1)
		require.Equal(t, "default", credentials[0].Env)
		require.Equal(t, "default-openai", credentials[0].Name)
		base, key, err := s.ResolveConnFromStore(ctx, env, &tenant, "openai", "", "")
		require.NoError(t, err)
		require.Equal(t, provider.URL+"/v1", base)
		require.Equal(t, "fixture-key", key)
		result, err := s.SyncOnlineModels(ctx, tenant, OnlineModelSyncRequest{Env: env, Provider: "openai", Modality: "llm", Action: "refresh"})
		require.NoError(t, err)
		require.Equal(t, "dev", result.Env)
	}
	require.Equal(t, 2, calls)
	after, err := s.credRepo.FindByScopeNameProvider(ctx, "default", &tenant, "default-openai", "openai")
	require.NoError(t, err)
	afterJSON, _ := json.Marshal(after.Data)
	require.Equal(t, string(before), string(afterJSON))
	_, err = s.credRepo.FindByScopeNameProvider(ctx, "dev", &tenant, "dev-openai", "openai")
	require.Error(t, err, "catalog refresh must not silently copy or migrate credentials")
	var keys int64
	require.NoError(t, s.db.Table("iam_tenant_key_pairs").Where("tenant_uuid=? AND env=?", tenant, "dev").Count(&keys).Error)
	require.Zero(t, keys, "read-only compatibility must not create a new encryption key")
	for _, scope := range []struct{ tenant, env string }{{tenant, "prod"}, {tenant, "staging"}, {"22222222-2222-4222-8222-222222222222", "dev"}} {
		_, err := s.SyncOnlineModels(ctx, scope.tenant, OnlineModelSyncRequest{Env: scope.env, Provider: "openai", Modality: "llm", Action: "refresh"})
		require.ErrorContains(t, err, "AI_SETTINGS_PROVIDER_NOT_CONFIGURED")
	}
	require.Equal(t, 2, calls)
}

func TestCanonicalProviderRecordWinsWithoutCombiningSecretsOrParameters(t *testing.T) {
	s := environmentAliasFixture(t)
	tenant := onlineTestTenant
	ctx := context.Background()
	legacyCalls, devCalls := 0, 0
	old := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { legacyCalls++; _, _ = w.Write([]byte(`{"data":[]}`)) }))
	defer old.Close()
	current := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		devCalls++
		require.Equal(t, "Bearer fixture-key", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer current.Close()
	savedOnlineCredential(t, s, tenant, "default", "openai", old.URL+"/v1")
	savedOnlineCredential(t, s, tenant, "dev", "openai", current.URL+"/v1")
	for _, record := range []am.AIModelProfile{
		{Env: "default", TenantUUID: &tenant, Modality: "llm", Provider: "openai", Model: "gpt-4o", Label: "legacy", Defaults: datatypes.JSONMap{"temperature": 0.12, "legacy_only": true}},
		{Env: "dev", TenantUUID: &tenant, Modality: "llm", Provider: "openai", Model: "gpt-4o", Label: "current", Defaults: datatypes.JSONMap{"temperature": 0.83}},
	} {
		require.NoError(t, s.db.Create(&record).Error)
	}
	require.NoError(t, s.db.Create(&am.AIRoutePolicy{Env: "default", TenantUUID: &tenant, Modality: "llm", Provider: "openai", Model: "gpt-4o"}).Error)
	for _, env := range []string{"default", "dev"} {
		credentials, err := s.ListCredentials(ctx, env, &tenant)
		require.NoError(t, err)
		require.Len(t, credentials, 1)
		require.Equal(t, "dev-openai", credentials[0].Name)
		profiles, err := s.ListProfiles(ctx, env, &tenant, "llm")
		require.NoError(t, err)
		require.Len(t, profiles, 1)
		require.Equal(t, "current", profiles[0].Label)
		require.NotContains(t, profiles[0].Defaults, "legacy_only")
		active, err := s.GetActiveProfile(ctx, env, &tenant, "llm")
		require.NoError(t, err)
		require.Equal(t, "current", active.Label)
		_, err = s.SyncOnlineModels(ctx, tenant, OnlineModelSyncRequest{Env: env, Provider: "openai", Action: "refresh", Modality: "llm"})
		require.NoError(t, err)
	}
	require.Equal(t, 2, devCalls)
	require.Zero(t, legacyCalls)
	// dev 记录存在但无效时显式失败，不能偷偷借用 default 的 Key 或地址。
	require.NoError(t, s.db.Model(&am.AIProviderCredential{}).Where("env=? AND tenant_uuid=?", "dev", tenant).Update("data", datatypes.JSONMap{"base_url": current.URL + "/v1"}).Error)
	_, err := s.SyncOnlineModels(ctx, tenant, OnlineModelSyncRequest{Env: "dev", Provider: "openai", Action: "refresh", Modality: "llm"})
	require.ErrorContains(t, err, "AI_MODEL_DISCOVERY_CONFIG_INVALID")
	require.Zero(t, legacyCalls)
	require.Equal(t, 2, devCalls)
}

func TestLegacyProfilesAndSecretsSurviveCanonicalSaveWithoutRetypingKey(t *testing.T) {
	s := environmentAliasFixture(t)
	tenant := onlineTestTenant
	ctx := context.Background()
	savedOnlineCredential(t, s, tenant, "default", "openai", "https://fixture.invalid/v1")
	// 未知敏感字段也必须保持密封，不能因为规范环境改变而落成普通 JSON。
	extra, err := s.tks.SealSensitive(ctx, "default", tenant, datatypes.JSONMap{"api_key": "fixture-key", "custom_secret": "fixture-custom-secret", "base_url": "https://fixture.invalid/v1"}, "api_key", "custom_secret")
	require.NoError(t, err)
	require.NoError(t, s.db.Model(&am.AIProviderCredential{}).Where("tenant_uuid=? AND env=?", tenant, "default").Update("data", extra).Error)
	old, err := s.credRepo.FindByScopeNameProvider(ctx, "default", &tenant, "default-openai", "openai")
	require.NoError(t, err)
	oldData, _ := json.Marshal(old.Data)
	require.NoError(t, s.db.Create(&am.AIModelProfile{Env: "default", TenantUUID: &tenant, Modality: "llm", Provider: "openai", Model: "gpt-4o", Defaults: datatypes.JSONMap{"temperature": 0.23, "maxTokens": 3000}}).Error)
	require.NoError(t, s.db.Create(&am.AIRoutePolicy{Env: "default", TenantUUID: &tenant, Modality: "llm", Provider: "openai", Model: "gpt-4o"}).Error)
	profile, err := s.GetActiveProfile(ctx, "dev", &tenant, "llm")
	require.NoError(t, err)
	require.Equal(t, "0.23", fmt.Sprint(profile.Defaults["temperature"]))
	credential := &am.AIProviderCredential{Name: "default-openai", Provider: "openai", AuthScheme: "bearer", Data: datatypes.JSONMap{"api_key": "••••••••"}}
	require.NoError(t, s.SaveCredentialOnly(ctx, "default", &tenant, credential))
	require.Equal(t, "dev", credential.Env)
	require.Equal(t, "dev-openai", credential.Name)
	canonical, err := s.credRepo.FindByScopeNameProvider(ctx, "dev", &tenant, "dev-openai", "openai")
	require.NoError(t, err)
	secrets := map[string]any{}
	require.NoError(t, s.tks.UnsealSensitive(ctx, "dev", tenant, canonical.Data, &secrets))
	require.Equal(t, "fixture-key", secrets["api_key"])
	require.Equal(t, "fixture-custom-secret", secrets["custom_secret"])
	require.NotContains(t, canonical.Data, "custom_secret")
	unchanged, err := s.credRepo.FindByScopeNameProvider(ctx, "default", &tenant, "default-openai", "openai")
	require.NoError(t, err)
	data, _ := json.Marshal(unchanged.Data)
	require.Equal(t, string(oldData), string(data))
	for _, env := range []string{"dev", "default"} {
		_, key, err := s.ResolveConnFromStore(ctx, env, &tenant, "openai", "", "")
		require.NoError(t, err)
		require.Equal(t, "fixture-key", key)
	}
	// 完整保存同样保留已有 Key，并把新画像和默认路由写到规范环境。
	require.NoError(t, s.SaveCredentialAndProfile(ctx, "default", &tenant, &am.AIProviderCredential{Name: "default-openai", Provider: "openai", Data: datatypes.JSONMap{}}, &am.AIModelProfile{Modality: "llm", Provider: "openai", Model: "gpt-4o", Defaults: datatypes.JSONMap{"temperature": 0.31}}, true))
	active, err := s.GetActiveProfile(ctx, "dev", &tenant, "llm")
	require.NoError(t, err)
	require.Equal(t, "dev", active.Env)
	require.Equal(t, "0.31", fmt.Sprint(active.Defaults["temperature"]))
}

func TestLegacyOnlineSnapshotAndTenantEnvironmentUseCanonicalIdentity(t *testing.T) {
	s := environmentAliasFixture(t)
	tenant := onlineTestTenant
	ctx := context.Background()
	savedOnlineCredential(t, s, tenant, "default", "openai", "https://fixture.invalid/v1")
	connection, err := s.savedCatalogConnection(ctx, "default", tenant, "openai")
	require.NoError(t, err)
	legacy := storedOnlineCatalog{OnlineModelCatalog: OnlineModelCatalog{Env: "default", Provider: "openai", App: "", Protocol: "openai", Source: "https://fixture.invalid/v1/models", UpdatedAt: "2026-10-30T00:00:00Z", Revision: "legacy-revision", Models: []OnlineCatalogModel{{ID: "brand-new-id", Modalities: []string{}, ManualModalities: []string{}, Match: "unmatched"}}}, Fingerprint: connectionFingerprint(connection)}
	raw, _ := json.Marshal(legacy)
	require.NoError(t, s.settingRepo.Upsert(ctx, &setting.TenantSetting{TenantUUID: tenant, Key: onlineCatalogKey("default", "openai", ""), ValueJSON: raw}))
	result, err := s.SyncOnlineModels(ctx, tenant, OnlineModelSyncRequest{Env: "dev", Provider: "openai", Modality: "llm", Action: "assign", ConfirmUnknown: true, Revision: "legacy-revision", Models: []string{"brand-new-id"}})
	require.NoError(t, err)
	require.Equal(t, "dev", result.Env)
	for _, env := range []string{"default", "dev"} {
		models, err := s.ModelsForTenant(ctx, env, &tenant, "llm", "openai", "", "")
		require.NoError(t, err)
		require.Contains(t, models, "brand-new-id")
	}
	require.NoError(t, s.settingRepo.Upsert(ctx, &setting.TenantSetting{TenantUUID: tenant, Key: TenantSettingKeyAICurrentEnv, ValueJSON: datatypes.JSON(`"default"`)}))
	env, configured, err := s.GetTenantCurrentAIEnv(ctx, tenant)
	require.NoError(t, err)
	require.True(t, configured)
	require.Equal(t, "dev", env)
	require.NoError(t, s.SetTenantCurrentAIEnv(ctx, tenant, "default"))
	row, err := s.settingRepo.GetByTenantAndKey(ctx, tenant, TenantSettingKeyAICurrentEnv)
	require.NoError(t, err)
	require.JSONEq(t, `"dev"`, string(row.ValueJSON))
}

func TestEnvironmentAliasRotationKeepsEachEncryptionScopeAndOtherEnvironments(t *testing.T) {
	s := environmentAliasFixture(t)
	tenant := onlineTestTenant
	ctx := context.Background()
	before := map[string]string{}
	for _, env := range []string{"default", "dev", "prod"} {
		savedOnlineCredential(t, s, tenant, env, "openai", "https://fixture.invalid/v1")
		record, err := s.credRepo.FindByScopeNameProvider(ctx, env, &tenant, env+"-openai", "openai")
		require.NoError(t, err)
		encoded, _ := json.Marshal(record.Data)
		before[env] = string(encoded)
	}
	require.NoError(t, s.RotateTenantCredentials(ctx, "default", &tenant))
	for _, env := range []string{"default", "dev", "prod"} {
		record, err := s.credRepo.FindByScopeNameProvider(ctx, env, &tenant, env+"-openai", "openai")
		require.NoError(t, err)
		encoded, _ := json.Marshal(record.Data)
		if env == "prod" {
			require.Equal(t, before[env], string(encoded))
		} else {
			require.NotEqual(t, before[env], string(encoded))
		}
		secrets := map[string]any{}
		require.NoError(t, s.tks.UnsealSensitive(ctx, env, tenant, record.Data, &secrets))
		require.Equal(t, "fixture-key", secrets["api_key"])
	}
}
