package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/mattn/go-sqlite3"
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
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestVLMSettingsHTTPReusesCredentialAndPreservesGenerationProfile(t *testing.T) {
	oldKey, oldSchema, oldReg := crypto.GetGlobalKeyB64(), core.PowerXSchema, catalog.GlobalAIRegister
	require.NoError(t, crypto.SetGlobalKeyB64(base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))))
	core.PowerXSchema = "main"
	reg := catalog.NewRegistry()
	require.NoError(t, reg.LoadByConfig(catalog.CatalogConfig{Dirs: []string{"../../../../../config/agents/providers.d"}, FailIfEmpty: true}, nil))
	catalog.GlobalAIRegister = reg
	t.Cleanup(func() {
		require.NoError(t, crypto.SetGlobalKeyB64(oldKey))
		core.PowerXSchema = oldSchema
		catalog.GlobalAIRegister = oldReg
	})
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	conn, err := pool.Conn(context.Background())
	require.NoError(t, err)
	require.NoError(t, conn.Raw(func(raw any) error {
		return raw.(*sqlite3.SQLiteConn).RegisterFunc("NOW", func() string { return time.Now().UTC().Format("2006-01-02 15:04:05") }, false)
	}))
	require.NoError(t, conn.Close())
	t.Cleanup(func() { _ = pool.Close() })
	for _, statement := range []string{
		`CREATE TABLE iam_tenant_key_pairs(id integer primary key,created_at datetime,updated_at datetime,deleted_at datetime,env text,tenant_uuid text,k_id text,alg text,public_pem text,enc_private text,active boolean)`,
		`CREATE TABLE ai_provider_credentials(id integer primary key,created_at datetime,updated_at datetime,deleted_at datetime,env text,tenant_uuid text,name text,provider text,auth_scheme text,data text)`,
		`CREATE UNIQUE INDEX credential_tenant ON ai_provider_credentials(env,tenant_uuid,name,provider)`,
		`CREATE TABLE ai_model_profiles(id integer primary key,created_at datetime,updated_at datetime,deleted_at datetime,env text,tenant_uuid text,modality text,provider text,model text,label text,defaults text,cap_cache text,tags text)`,
		`CREATE UNIQUE INDEX profile_tenant ON ai_model_profiles(env,tenant_uuid,modality,provider,model)`,
		`CREATE TABLE ai_route_policies(id integer primary key,created_at datetime,updated_at datetime,deleted_at datetime,env text,tenant_uuid text,modality text,agent_id text,flow_id text,purpose text,provider text,model text,strategy text,compliance text,quota text)`,
		`CREATE TABLE cfg_tenant_settings(id integer primary key,created_at datetime,updated_at datetime,deleted_at datetime,tenant_uuid text,key text,value_json text,"group" text,description text,editable boolean,UNIQUE(tenant_uuid,key))`,
	} {
		require.NoError(t, db.Exec(statement).Error)
	}
	tenant := "11111111-1111-4111-8111-111111111111"
	ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant), &reqctx.CoreXClaims{IsRoot: true, TenantUUID: tenant})
	svc := agentsvc.NewAgentSettingService(db)
	providerCalls := 0
	runAllowed, upstreamStatus, slow := false, 200, false
	slowFinished := make(chan struct{}, 1)
	expectedKey, expectedPath := "private-fixture", "/v1/chat/completions"
	var lastProviderBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			_, _ = w.Write([]byte(`{"data":[{"id":"gpt-6-luna"},{"id":"fresh-vision-native"}]}`))
			return
		}
		providerCalls++
		if !runAllowed {
			t.Error("save/parameter check must not invoke the provider")
			return
		}
		require.Equal(t, expectedPath, r.URL.Path)
		require.Equal(t, "Bearer "+expectedKey, r.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&lastProviderBody))
		if slow {
			defer func() { slowFinished <- struct{}{} }()
			time.Sleep(200 * time.Millisecond)
		}
		if upstreamStatus != 200 {
			w.Header().Set("X-Request-ID", "provider-trial-trace")
			w.WriteHeader(upstreamStatus)
			_, _ = w.Write([]byte(`{"error":{"code":"rate_limit","message":"fixture failure"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"A blue test picture."}}],"usage":{"total_tokens":12}}`))
	}))
	defer srv.Close()
	require.NoError(t, svc.SaveCredentialOnly(ctx, "dev", &tenant, &am.AIProviderCredential{Name: "dev-openai", Provider: "openai", Data: datatypes.JSONMap{"api_key": "private-fixture", "base_url": srv.URL + "/v1"}}))
	image := am.AIModelProfile{Env: "dev", TenantUUID: &tenant, Modality: "image", Provider: "openai", Model: "gpt-image-2", Defaults: datatypes.JSONMap{"size": "1024x1024"}}
	require.NoError(t, db.Create(&image).Error)
	require.NoError(t, svc.SetActiveProfile(ctx, "dev", &tenant, "image", "openai", "gpt-image-2"))
	var credBefore am.AIProviderCredential
	require.NoError(t, db.First(&credBefore).Error)
	h := &AgentSettingHandler{svc: svc, aiSvc: aisvc.NewService(db)}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Request = c.Request.WithContext(ctx) })
	router.POST("/save", h.saveSettings)
	router.POST("/check", h.testQuickCall)
	call := func(path, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest("POST", path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, request)
		return w
	}
	body := `{"env":"dev","modality":"vlm","vlm":{"provider":"openai","model":"gpt-6-luna","reuseCredential":true,"maxTokens":2048,"reasoningEffort":"none","jsonMode":false}}`
	response := call("/save", body)
	require.Equal(t, 200, response.Code, response.Body.String())
	active, err := svc.GetActiveProfile(ctx, "dev", &tenant, "vlm")
	require.NoError(t, err)
	require.Equal(t, "gpt-6-luna", active.Model)
	require.Equal(t, "2048", fmt.Sprint(active.Defaults["max_tokens"]))
	check := call("/check", body)
	require.Equal(t, 200, check.Code, check.Body.String())
	require.Contains(t, check.Body.String(), `"generation_submitted":false`)
	bad := strings.Replace(body, `"reasoningEffort":"none"`, `"reasoningEffort":"low","temperature":0`, 1)
	rejected := call("/save", bad)
	require.Equal(t, 422, rejected.Code)
	require.Contains(t, rejected.Body.String(), "requires_reasoning_effort_none")
	var credAfter am.AIProviderCredential
	require.NoError(t, db.First(&credAfter).Error)
	require.Equal(t, credBefore.Data, credAfter.Data)
	imageAfter, err := svc.GetActiveProfile(ctx, "dev", &tenant, "image")
	require.NoError(t, err)
	require.Equal(t, image.ID, imageAfter.ID)
	require.Equal(t, image.Model, imageAfter.Model)
	require.Equal(t, image.Defaults, imageAfter.Defaults)
	require.Zero(t, providerCalls)
	// 在线新型号不需要本地登记，也不需要重发 Provider 密钥。
	future := `{"env":"dev","modality":"vlm","vlm":{"provider":"openai","model":"fresh-vision-native","reuseCredential":true,"maxTokens":2048,"jsonMode":false}}`
	response = call("/save", future)
	require.Equal(t, 200, response.Code, response.Body.String())
	active, err = svc.GetActiveProfile(ctx, "dev", &tenant, "vlm")
	require.NoError(t, err)
	require.Equal(t, "fresh-vision-native", active.Model)
	require.Contains(t, active.Tags, catalog.OnlineUsageSelectedTag)
	require.NoError(t, db.First(&credAfter).Error)
	require.Equal(t, credBefore.Data, credAfter.Data)
	imageAfter, err = svc.GetActiveProfile(ctx, "dev", &tenant, "image")
	require.NoError(t, err)
	require.Equal(t, image.ID, imageAfter.ID)
	require.Zero(t, providerCalls, "saving an online model does not run inference")

	// 真实 HTTP 管理入口：只有 execute=true 才识别图片，引用已保存的同租户环境凭据。
	runAllowed = true
	trial := strings.TrimSuffix(future, "}") + `,"execute":true,"inputs":[{"type":"text","content":"Describe the picture"},{"type":"image_url","url":"https://example.invalid/picture.png"}]}`
	analysis := call("/check", trial)
	require.Equal(t, 200, analysis.Code, analysis.Body.String())
	require.Contains(t, analysis.Body.String(), `"generation_submitted":true`)
	require.Contains(t, analysis.Body.String(), `"type":"analysis"`)
	require.Contains(t, analysis.Body.String(), `"text":"A blue test picture."`)
	require.Contains(t, analysis.Body.String(), `"trace_id"`)
	require.Contains(t, analysis.Body.String(), `"elapsed_ms"`)
	require.Equal(t, "fresh-vision-native", lastProviderBody["model"])
	require.Equal(t, 1, providerCalls)
	png := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aU1cAAAAASUVORK5CYII="
	base64Trial := strings.Replace(trial, `"type":"image_url","url":"https://example.invalid/picture.png"`, `"type":"image_base64","content":"data:image/png;base64,`+png+`"`, 1)
	require.Equal(t, 200, call("/check", base64Trial).Code)
	encoded, _ := json.Marshal(lastProviderBody)
	require.Contains(t, string(encoded), "data:image/png;base64,")
	require.Equal(t, 2, providerCalls)
	require.NotContains(t, analysis.Body.String(), "private-fixture")
	// 没有显式执行、没有图片、非法地址或非法参数都不能提交推理。
	require.Contains(t, call("/check", strings.Replace(trial, `"execute":true`, `"execute":false`, 1)).Body.String(), `"generation_submitted":false`)
	require.Equal(t, 422, call("/check", strings.Replace(trial, `"type":"image_url"`, `"type":"video_url"`, 1)).Code)
	require.Equal(t, 422, call("/check", strings.Replace(trial, `"reuseCredential":true`, `"reuseCredential":true,"baseURL":"file://invalid"`, 1)).Code)
	badParam := call("/check", strings.Replace(trial, `"maxTokens":2048`, `"maxTokens":-1`, 1))
	require.Equal(t, 422, badParam.Code)
	require.Contains(t, badParam.Body.String(), `"parameter":"max_tokens"`)
	require.Equal(t, 2, providerCalls)
	originalContext := ctx
	ctx = reqctx.WithClaims(ctx, nil)
	require.Equal(t, 401, call("/check", trial).Code)
	ctx = originalContext
	require.Equal(t, 2, providerCalls)
	upstreamStatus = 429
	rejectedTrial := call("/check", trial)
	require.Equal(t, 429, rejectedTrial.Code, rejectedTrial.Body.String())
	require.Contains(t, rejectedTrial.Body.String(), `"upstream_status":429`)
	require.Contains(t, rejectedTrial.Body.String(), "provider-trial-trace")
	upstreamStatus, slow = 200, true
	h.visionTrialTimeout = 75 * time.Millisecond
	timedOut := call("/check", trial)
	require.Equal(t, 504, timedOut.Code, timedOut.Body.String())
	require.Contains(t, timedOut.Body.String(), "AI_PROVIDER_TIMEOUT")
	require.NoError(t, db.First(&credAfter).Error)
	require.Equal(t, credBefore.Data, credAfter.Data)
	imageAfter, err = svc.GetActiveProfile(ctx, "dev", &tenant, "image")
	require.NoError(t, err)
	require.Equal(t, image.ID, imageAfter.ID)
	require.Equal(t, image.Defaults, imageAfter.Defaults)
	// 当前表单试跑：新 Key、新地址、新型号均不要求先保存；不会创建任何配置记录。
	// 超时返回不代表 httptest 的慢请求已退出；完成后才能修改它读取的夹具变量。
	select {
	case <-slowFinished:
	case <-time.After(time.Second):
		t.Fatal("slow provider fixture did not finish")
	}
	slow, upstreamStatus, h.visionTrialTimeout = false, 200, 0
	countsBefore := map[string]int64{}
	for _, table := range []string{"ai_provider_credentials", "ai_model_profiles", "ai_route_policies", "cfg_tenant_settings", "iam_tenant_key_pairs"} {
		var count int64
		require.NoError(t, db.Table(table).Count(&count).Error)
		countsBefore[table] = count
	}
	newTenant := "22222222-2222-4222-8222-222222222222"
	ctx = reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), newTenant), &reqctx.CoreXClaims{IsRoot: true, TenantUUID: newTenant})
	expectedKey, expectedPath = "draft-fixture", "/draft/v1/chat/completions"
	draft := fmt.Sprintf(`{"env":"dev","modality":"vlm","execute":true,"useFormConnection":true,"vlm":{"provider":"openai","model":"unsaved-vision-model","apiKey":"draft-fixture","baseURL":%q,"maxTokens":3072},"inputs":[{"type":"text","content":"Describe"},{"type":"image_url","url":"https://example.invalid/test.png"}]}`, srv.URL+"/draft/v1")
	beforeCalls := providerCalls
	parameterOnly := call("/check", strings.Replace(draft, `"execute":true`, `"execute":false`, 1))
	require.Equal(t, 200, parameterOnly.Code, parameterOnly.Body.String())
	require.Contains(t, parameterOnly.Body.String(), `"generation_submitted":false`)
	require.Equal(t, beforeCalls, providerCalls)
	draftResponse := call("/check", draft)
	require.Equal(t, 200, draftResponse.Code, draftResponse.Body.String())
	require.Contains(t, draftResponse.Body.String(), `"type":"analysis"`)
	require.Equal(t, "unsaved-vision-model", lastProviderBody["model"])
	require.EqualValues(t, 3072, lastProviderBody["max_completion_tokens"])
	require.Equal(t, beforeCalls+1, providerCalls)
	require.NotContains(t, draftResponse.Body.String(), "draft-fixture")
	// 未填 Key 时，仅复用本租户环境的 Key，仍使用表单里的临时地址和新型号。
	ctx, expectedKey = originalContext, "private-fixture"
	withoutKey := strings.Replace(draft, `"apiKey":"draft-fixture",`, "", 1)
	require.Equal(t, 200, call("/check", withoutKey).Code)
	// 其他租户和生产环境均不能借用现有开发 Key。
	ctx = reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), newTenant), &reqctx.CoreXClaims{IsRoot: true, TenantUUID: newTenant})
	missingKey := call("/check", withoutKey)
	require.Equal(t, 422, missingKey.Code)
	require.Contains(t, missingKey.Body.String(), "AI_CONNECTION_CONFIG_INVALID")
	ctx = originalContext
	require.Equal(t, 422, call("/check", strings.Replace(withoutKey, `"env":"dev"`, `"env":"prod"`, 1)).Code)
	require.Equal(t, beforeCalls+2, providerCalls)
	for table, before := range countsBefore {
		var after int64
		require.NoError(t, db.Table(table).Count(&after).Error)
		require.Equal(t, before, after, table)
	}
	require.NoError(t, db.First(&credAfter).Error)
	require.Equal(t, credBefore.Data, credAfter.Data)
	activeAfterTrial, err := svc.GetActiveProfile(ctx, "dev", &tenant, "vlm")
	require.NoError(t, err)
	require.Equal(t, "fresh-vision-native", activeAfterTrial.Model)

}
