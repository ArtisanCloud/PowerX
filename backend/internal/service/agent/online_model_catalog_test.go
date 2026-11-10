package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	am "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/ArtisanCloud/PowerX/pkg/crypto"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	glog "gorm.io/gorm/logger"
)

const onlineTestTenant = "11111111-1111-4111-8111-111111111111"

func onlineCatalogFixture(t *testing.T) (*AgentSettingService, *catalog.Registry) {
	t.Helper()
	previousKey := crypto.GetGlobalKeyB64()
	require.NoError(t, crypto.SetGlobalKeyB64(base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))))
	t.Cleanup(func() { require.NoError(t, crypto.SetGlobalKeyB64(previousKey)) })
	previousSchema := core.PowerXSchema
	previousRegistry := catalog.GlobalAIRegister
	core.PowerXSchema = "main"
	reg := catalog.NewRegistry()
	require.NoError(t, reg.LoadByConfig(catalog.CatalogConfig{Dirs: []string{"../../../config/agents/providers.d"}, FailIfEmpty: true}, nil))
	catalog.GlobalAIRegister = reg
	t.Cleanup(func() { core.PowerXSchema = previousSchema; catalog.GlobalAIRegister = previousRegistry })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: glog.Default.LogMode(glog.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = pool.Close() })
	for _, sql := range []string{
		`CREATE TABLE iam_tenant_key_pairs(id integer primary key, created_at datetime, updated_at datetime, deleted_at datetime, env text, tenant_uuid text, k_id text, alg text, public_pem text, enc_private text, active boolean)`,
		`CREATE TABLE ai_provider_credentials(id integer primary key, created_at datetime, updated_at datetime, deleted_at datetime, env text, tenant_uuid text, name text, provider text, auth_scheme text, data text)`,
		`CREATE TABLE cfg_tenant_settings(id integer primary key, created_at datetime, updated_at datetime, deleted_at datetime, tenant_uuid text, key text, value_json text, "group" text, description text, editable boolean, UNIQUE(tenant_uuid,key))`,
		`CREATE TABLE ai_model_profiles(id integer primary key, created_at datetime, updated_at datetime, deleted_at datetime, env text, tenant_uuid text, modality text, provider text, model text, label text, defaults text, cap_cache text, tags text)`,
	} {
		require.NoError(t, db.Exec(sql).Error)
	}
	return NewAgentSettingService(db), reg
}

func savedOnlineCredential(t *testing.T, s *AgentSettingService, tenant, env, provider, endpoint string) {
	t.Helper()
	data, err := s.tks.SealSensitive(context.Background(), env, tenant, datatypes.JSONMap{"api_key": "fixture-key", "base_url": endpoint}, "api_key")
	require.NoError(t, err)
	require.NoError(t, s.db.Create(&am.AIProviderCredential{Env: env, TenantUUID: &tenant, Name: utils.Slug(env + "-" + provider), Provider: provider, AuthScheme: "bearer", Data: data}).Error)
}

func TestOnlineCatalogPersistsMatchesAssignmentsAndRuntimeLookupWithoutGlobalMutation(t *testing.T) {
	s, reg := onlineCatalogFixture(t)
	manifest, _ := reg.Manifest("openai")
	original := len(manifest.Modalities["llm"].Models)
	status := http.StatusOK
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "/v1/models", r.URL.Path)
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "Bearer fixture-key", r.Header.Get("Authorization"))
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o"},{"id":"brand-new-id"},{"id":"provider-text","output_modalities":["text"]}]}`))
	}))
	defer srv.Close()
	savedOnlineCredential(t, s, onlineTestTenant, "dev", "openai", srv.URL+"/v1")
	ctx := context.Background()
	request := OnlineModelSyncRequest{Env: "dev", Provider: "OpenAI", Modality: "llm", Action: "refresh"}
	result, err := s.SyncOnlineModels(ctx, onlineTestTenant, request)
	require.NoError(t, err)
	require.Len(t, result.Models, 3)
	require.Equal(t, "local", result.Models[0].Match)
	require.Equal(t, "unmatched", result.Models[1].Match)
	require.Equal(t, "provider", result.Models[2].Match)
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "fixture-key")
	require.NotContains(t, string(raw), "connection_fingerprint")
	tenant := onlineTestTenant
	models, err := s.ModelsForTenant(ctx, "dev", &tenant, "llm", "openai", "", "")
	require.NoError(t, err)
	require.Contains(t, models, "provider-text")
	require.Contains(t, models, "brand-new-id", "online IDs are selectable without local registration")
	require.Equal(t, 1, calls, "local reads must not fetch a provider catalog")
	require.NoError(t, s.ensureTenantModelExists(ctx, "dev", &tenant, "llm", "openai", "brand-new-id"))
	assignment := OnlineModelSyncRequest{Env: "dev", Provider: "openai", Action: "assign", ConfirmUnknown: true, Modality: "llm", Revision: result.Revision, Models: []string{"brand-new-id"}}
	assigned, err := s.SyncOnlineModels(ctx, tenant, assignment)
	require.NoError(t, err)
	require.NotEqual(t, result.Revision, assigned.Revision)
	require.Equal(t, "manual", assigned.Models[1].Match)
	models, err = NewAgentSettingService(s.db).ModelsForTenant(ctx, "dev", &tenant, "llm", "openai", "", "")
	require.NoError(t, err)
	require.Contains(t, models, "brand-new-id")
	require.NoError(t, s.ensureTenantModelExists(ctx, "dev", &tenant, "llm", "openai", "brand-new-id"))
	_, err = s.BuildModelConfig(ctx, "dev", &tenant, "llm", "openai", "brand-new-id", "", "", "", "", "", "")
	require.NoError(t, err, "a saved online candidate must survive runtime model validation")
	require.NoError(t, s.ensureTenantModelExists(ctx, "dev", &tenant, "embedding", "openai", "brand-new-id"))
	require.Error(t, s.ensureTenantModelExists(ctx, "prod", &tenant, "llm", "openai", "brand-new-id"))
	other := "22222222-2222-4222-8222-222222222222"
	savedOnlineCredential(t, s, other, "dev", "openai", srv.URL+"/v1")
	require.NoError(t, s.ensureTenantModelExists(ctx, "dev", &other, "llm", "openai", "brand-new-id"), "another tenant fetches its own authorized connection")
	require.Equal(t, original, len(manifest.Modalities["llm"].Models))
	require.Nil(t, findModelManifest("llm", "openai", "brand-new-id"), "tenant sync must not mutate global YAML registry")
	_, err = s.SyncOnlineModels(ctx, tenant, assignment)
	require.ErrorContains(t, err, "AI_MODEL_SYNC_STALE")
	status = http.StatusBadGateway
	_, err = s.SyncOnlineModels(ctx, tenant, request)
	require.ErrorContains(t, err, "AI_MODEL_DISCOVERY_UPSTREAM_FAILED")
	item, err := s.settingRepo.GetByTenantAndKey(ctx, tenant, onlineCatalogKey("dev", "openai", ""))
	require.NoError(t, err)
	snapshot, err := decodeOnlineCatalog(item)
	require.NoError(t, err)
	require.Equal(t, assigned.Revision, snapshot.Revision, "failed refresh must preserve the previous saved result")
	status = http.StatusOK
	refreshed, err := s.SyncOnlineModels(ctx, tenant, request)
	require.NoError(t, err)
	require.Equal(t, "manual", refreshed.Models[1].Match, "refresh retains explicit assignments for the same model and connection")
}

func TestOnlineCatalogRejectsStaleConnectionUnsupportedProviderAndRollsBackSaveFailure(t *testing.T) {
	s, _ := onlineCatalogFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"brand-new-id"}]}`))
	}))
	defer srv.Close()
	tenant := onlineTestTenant
	savedOnlineCredential(t, s, tenant, "dev", "openai", srv.URL+"/v1")
	ctx := context.Background()
	request := OnlineModelSyncRequest{Env: "dev", Provider: "openai", Modality: "llm", Action: "refresh"}
	result, err := s.SyncOnlineModels(ctx, tenant, request)
	require.NoError(t, err)
	for _, assignment := range []OnlineModelSyncRequest{
		{Env: "dev", Provider: "openai", Action: "assign", ConfirmUnknown: true, Modality: "image", Revision: result.Revision, Models: []string{"brand-new-id"}},
		{Env: "dev", Provider: "openai", Action: "assign", ConfirmUnknown: true, Modality: "llm", Revision: result.Revision, Models: []string{"not-discovered"}},
	} {
		_, err := s.SyncOnlineModels(ctx, tenant, assignment)
		require.Error(t, err)
	}
	_, err = s.SyncOnlineModels(ctx, tenant, OnlineModelSyncRequest{Env: "dev", Provider: "agnes", Action: "refresh"})
	require.ErrorContains(t, err, "AI_MODEL_SYNC_UNSUPPORTED")
	_, err = s.SyncOnlineModels(ctx, tenant, OnlineModelSyncRequest{Env: "prod", Provider: "openai", Action: "refresh"})
	require.ErrorContains(t, err, "AI_SETTINGS_PROVIDER_NOT_CONFIGURED")
	require.NoError(t, s.db.Callback().Create().Before("gorm:create").Register("online_catalog_save_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "cfg_tenant_settings" {
			tx.AddError(errors.New("fixture write failure"))
		}
	}))
	_, err = s.SyncOnlineModels(ctx, tenant, request)
	require.ErrorContains(t, err, "fixture write failure")
	require.NoError(t, s.db.Callback().Create().Remove("online_catalog_save_failure"))
	item, err := s.settingRepo.GetByTenantAndKey(ctx, tenant, onlineCatalogKey("dev", "openai", ""))
	require.NoError(t, err)
	snapshot, err := decodeOnlineCatalog(item)
	require.NoError(t, err)
	require.Equal(t, result.Revision, snapshot.Revision)
	assignment := OnlineModelSyncRequest{Env: "dev", Provider: "openai", Action: "assign", ConfirmUnknown: true, Modality: "llm", Revision: result.Revision, Models: []string{"brand-new-id"}}
	_, err = s.SyncOnlineModels(ctx, tenant, assignment)
	require.NoError(t, err)
	rotated, err := s.tks.SealSensitive(ctx, "dev", tenant, datatypes.JSONMap{"api_key": "rotated-fixture-key", "base_url": srv.URL + "/v1"}, "api_key")
	require.NoError(t, err)
	require.NoError(t, s.db.Model(&am.AIProviderCredential{}).Where("tenant_uuid = ? AND provider = ?", tenant, "openai").Update("data", rotated).Error)
	require.NoError(t, s.ensureTenantModelExists(ctx, "dev", &tenant, "llm", "openai", "brand-new-id"), "a different key fetches a new catalog through its own connection")
}

func TestCompatibleCatalogPreservesOptionalSavedKeyAndInjectedTimeout(t *testing.T) {
	s, _ := onlineCatalogFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer fixture-key", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"data":[{"id":"custom-text"}]}`))
	}))
	defer srv.Close()
	savedOnlineCredential(t, s, onlineTestTenant, "dev", "openai_compatible", srv.URL+"/v1")
	configured := NewAgentSettingServiceWithCatalogTimeout(s.db, 37*time.Second)
	in, err := configured.savedCatalogConnection(context.Background(), "dev", onlineTestTenant, "openai_compatible")
	require.NoError(t, err)
	require.Equal(t, "fixture-key", in.APIKey)
	require.Equal(t, 37*time.Second, in.Timeout)
	prior := connectionFingerprint(in)
	in.Timeout = time.Second
	require.Equal(t, prior, connectionFingerprint(in), "query timeout is not credential identity")
	_, err = configured.SyncOnlineModels(context.Background(), onlineTestTenant, OnlineModelSyncRequest{Provider: "openai_compatible", Env: "dev", Modality: "llm", Action: "refresh"})
	require.NoError(t, err)
}
