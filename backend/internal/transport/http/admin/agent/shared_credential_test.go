package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	am "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentsvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	aisvc "github.com/ArtisanCloud/PowerX/internal/service/ai"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/crypto"
	"github.com/gin-gonic/gin"
	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSharedProviderCredentialFromLLMToImage(t *testing.T) {
	oldKey, oldSchema, oldRegistry := crypto.GetGlobalKeyB64(), core.PowerXSchema, catalog.GlobalAIRegister
	require.NoError(t, crypto.SetGlobalKeyB64(base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))))
	core.PowerXSchema = "main"
	registry := catalog.NewRegistry()
	require.NoError(t, registry.LoadByConfig(catalog.CatalogConfig{Dirs: []string{"../../../../../config/agents/providers.d"}, FailIfEmpty: true}, nil))
	catalog.GlobalAIRegister = registry
	t.Cleanup(func() {
		require.NoError(t, crypto.SetGlobalKeyB64(oldKey))
		core.PowerXSchema, catalog.GlobalAIRegister = oldSchema, oldRegistry
	})
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = pool.Close() })
	conn, err := pool.Conn(context.Background())
	require.NoError(t, err)
	require.NoError(t, conn.Raw(func(raw any) error {
		return raw.(*sqlite3.SQLiteConn).RegisterFunc("NOW", func() string { return time.Now().UTC().Format("2006-01-02 15:04:05") }, false)
	}))
	require.NoError(t, conn.Close())
	for _, statement := range []string{
		`CREATE TABLE iam_tenant_key_pairs(id integer primary key,created_at datetime,updated_at datetime,deleted_at datetime,env text,tenant_uuid text,k_id text,alg text,public_pem text,enc_private text,active boolean)`,
		`CREATE TABLE ai_provider_credentials(id integer primary key,created_at datetime,updated_at datetime,deleted_at datetime,env text,tenant_uuid text,name text,provider text,auth_scheme text,data text,UNIQUE(env,tenant_uuid,name,provider))`,
		`CREATE TABLE ai_model_profiles(id integer primary key,created_at datetime,updated_at datetime,deleted_at datetime,env text,tenant_uuid text,modality text,provider text,model text,label text,defaults text,cap_cache text,tags text,UNIQUE(env,tenant_uuid,modality,provider,model))`,
		`CREATE TABLE ai_route_policies(id integer primary key,created_at datetime,updated_at datetime,deleted_at datetime,env text,tenant_uuid text,modality text,agent_id text,flow_id text,purpose text,provider text,model text,strategy text,compliance text,quota text)`,
		`CREATE TABLE cfg_tenant_settings(id integer primary key,created_at datetime,updated_at datetime,deleted_at datetime,tenant_uuid text,key text,value_json text,"group" text,description text,editable boolean,UNIQUE(tenant_uuid,key))`,
	} {
		require.NoError(t, db.Exec(statement).Error)
	}
	tenant := "11111111-1111-4111-8111-111111111111"
	ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant), &reqctx.CoreXClaims{IsRoot: true, TenantUUID: tenant})
	keys := []string{}
	generationCalls := 0
	expectedKey := "shared-fixture-key"
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+expectedKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		keys = append(keys, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o"},{"id":"gpt-image-1"}]}`))
			return
		}
		require.Equal(t, "/v1/images/generations", r.URL.Path)
		generationCalls++
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"aW1hZ2UtZml4dHVyZQ=="}]}`))
	}))
	defer provider.Close()
	svc := agentsvc.NewAgentSettingService(db)
	handler := &AgentSettingHandler{svc: svc}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Request = c.Request.WithContext(ctx) })
	router.POST("/save", handler.saveSettings)
	router.POST("/test", handler.testConnection)
	router.GET("/credentials", handler.listCredentials)
	call := func(method, path string, body any) *httptest.ResponseRecorder {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	llm := map[string]any{"provider": "openai", "model": "gpt-4o", "apiKey": "shared-fixture-key", "baseURL": provider.URL + "/v1", "maxTokens": 64}
	res := call("POST", "/save", map[string]any{"env": "dev", "modality": "llm", "llm": llm})
	require.Equal(t, 200, res.Code, res.Body.String())
	var before am.AIProviderCredential
	require.NoError(t, db.Where("provider = ?", "openai").First(&before).Error)
	image := map[string]any{"provider": "openai", "model": "gpt-image-1", "apiKey": "", "baseURL": provider.URL + "/v1", "size": "1024x1024", "quality": "auto", "format": "png"}
	for _, path := range []string{"/test", "/save"} {
		res := call("POST", path, map[string]any{"env": "dev", "modality": "image", "image": image})
		require.Equal(t, 200, res.Code, res.Body.String())
	}
	require.Zero(t, generationCalls, "保存及连接测试不提交生成请求")
	result, err := aisvc.NewService(db).ImageInvoke(ctx, "dev", tenant, "openai/gpt-image-1", []aisvc.ContentItem{{Type: "text", Content: "A fixture square"}}, nil)
	require.NoError(t, err)
	require.Equal(t, "image", result["type"])
	require.Equal(t, 1, generationCalls)
	for _, key := range keys {
		require.Equal(t, "Bearer shared-fixture-key", key)
	}
	var after am.AIProviderCredential
	require.NoError(t, db.First(&after).Error)
	require.Equal(t, before.Data["__sealed"], after.Data["__sealed"], "图像留空复用，不能替换共享密文")
	var count int64
	require.NoError(t, db.Model(&am.AIProviderCredential{}).Count(&count).Error)
	require.EqualValues(t, 1, count, "不同模态不能各建一份同 Provider 凭据")
	res = call("GET", "/credentials?env=dev", nil)
	require.Equal(t, 200, res.Code)
	require.Contains(t, res.Body.String(), `"has_credentials":true`)
	require.NotContains(t, res.Body.String(), "shared-fixture-key")
	require.NotContains(t, res.Body.String(), "__sealed")
	res = call("GET", "/credentials?env=prod", nil)
	require.NotContains(t, res.Body.String(), `"has_credentials":true`)
	// 其他租户、环境、Provider 均不能继承这份共享 Key。
	otherTenant := "22222222-2222-4222-8222-222222222222"
	for _, scope := range []struct{ env, tenant, provider string }{{"prod", tenant, "openai"}, {"dev", otherTenant, "openai"}, {"dev", tenant, "deepseek"}} {
		_, _, err := svc.ResolveConnFromStore(ctx, scope.env, &scope.tenant, scope.provider, "", "")
		require.Error(t, err)
	}
	// 新 Key 的既有语义是更新共享连接，不能向用户暗示这是图像专用覆盖。
	expectedKey = "replacement-fixture-key"
	image["apiKey"] = expectedKey
	res = call("POST", "/test", map[string]any{"env": "dev", "modality": "image", "image": image})
	require.Equal(t, 200, res.Code, res.Body.String())
	base, key, err := svc.ResolveConnFromStore(ctx, "dev", &tenant, "openai", "", "")
	require.NoError(t, err)
	require.Equal(t, expectedKey, key)
	require.Equal(t, provider.URL+"/v1", base)
	llmProfile, err := svc.GetActiveProfile(ctx, "dev", &tenant, "llm")
	require.NoError(t, err)
	require.Equal(t, "gpt-4o", llmProfile.Model, "更新共享 Key 不修改其他模态的型号")
	// 即使带新凭据，非法图像默认值也必须先返回 422，不能部分写入或连接供应商。
	beforeCalls := len(keys)
	image["model"], image["quality"], image["apiKey"] = "gpt-image-2", "standard", "must-not-be-saved"
	res = call("POST", "/save", map[string]any{"env": "dev", "modality": "image", "image": image})
	require.Equal(t, 422, res.Code, res.Body.String())
	require.Contains(t, res.Body.String(), "AI_MODEL_DEFAULTS_INVALID")
	require.Contains(t, res.Body.String(), `"parameter":"quality"`)
	require.Contains(t, res.Body.String(), `"allowed_values":["auto","low","medium","high"]`)
	require.Len(t, keys, beforeCalls)
	_, key, err = svc.ResolveConnFromStore(ctx, "dev", &tenant, "openai", "", "")
	require.NoError(t, err)
	require.Equal(t, expectedKey, key)
}

func TestCredentialSummaryRemovesLegacyAndSealedSecretsWithoutMutatingInput(t *testing.T) {
	data := datatypes.JSONMap{"api_key": "legacy-secret", "apiKey": "legacy-alias", "secret_key": "secret-access", "secretKey": "secret-alias", "access_token": "access-secret", "client_secret": "client-secret", "secret": "generic-secret", "__sealed": map[string]any{"ct": "ciphertext"}, "base_url": "https://provider.example.invalid"}
	credential := am.AIProviderCredential{Data: data}
	redactCredentialSecrets(&credential)
	require.Equal(t, datatypes.JSONMap{"base_url": "https://provider.example.invalid"}, credential.Data)
	require.Len(t, data, 9, "响应脱敏不能改写原始凭据快照")
}
