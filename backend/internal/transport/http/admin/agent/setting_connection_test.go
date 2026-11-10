package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	agentsvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestConnectionHTTPWithoutModelReachesReadOnlyProbe(t *testing.T) {
	previous := catalog.GlobalAIRegister
	reg := catalog.NewRegistry()
	require.NoError(t, reg.LoadByConfig(catalog.CatalogConfig{Dirs: []string{"../../../../../config/agents/providers.d"}, FailIfEmpty: true}, nil))
	catalog.GlobalAIRegister = reg
	t.Cleanup(func() { catalog.GlobalAIRegister = previous })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Close() })
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/tags", r.URL.Path)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	manifest, _ := reg.Manifest("ollama")
	manifest.Auth.Defaults["base_url"] = srv.URL
	h := &AgentSettingHandler{svc: agentsvc.NewAgentSettingService(db)}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(reqctx.WithTenantUUID(c.Request.Context(), "f23491d2-b8cd-4107-a17b-301a44168aa6"))
	})
	router.POST("/test/connection", h.testConnection)
	router.POST("/test/call", h.testQuickCall)
	router.POST("/settings/save", h.saveSettings)
	body := `{"env":"dev","modality":"llm","llm":{"provider":"ollama","model":"","baseURL":"","apiKey":""}}`
	for _, path := range []string{"/test/connection", "/test/call", "/settings/save"} {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if path == "/test/connection" {
			// 故意让上游拒绝：证明空型号请求已越过绑定校验并抵达只读接口，而非本地伪造成功。
			require.Equal(t, 422, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), "AI_CONNECTION_AUTH_FAILED")
			require.NotContains(t, response.Body.String(), "required")
		} else {
			require.Equal(t, 400, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), "Model")
			require.Contains(t, response.Body.String(), "required")
		}
	}
	require.EqualValues(t, 1, calls.Load(), "saving/running an empty model must not reach Ollama")
}

func TestConnectionHTTPClearedBaseURLDoesNotSendNewKeyToStoredProxy(t *testing.T) {
	previous, schema := catalog.GlobalAIRegister, coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	reg := catalog.NewRegistry()
	require.NoError(t, reg.LoadByConfig(catalog.CatalogConfig{Dirs: []string{"../../../../../config/agents/providers.d"}, FailIfEmpty: true}, nil))
	catalog.GlobalAIRegister = reg
	t.Cleanup(func() { catalog.GlobalAIRegister = previous; coremodel.PowerXSchema = schema })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Close() })
	require.NoError(t, db.Exec(`CREATE TABLE ai_provider_credentials(id integer primary key, created_at datetime, updated_at datetime, deleted_at datetime, env text, tenant_uuid text, name text, provider text, data text)`).Error)
	var oldCalls, defaultCalls atomic.Int32
	old := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { oldCalls.Add(1); w.WriteHeader(http.StatusForbidden) }))
	defer old.Close()
	current := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defaultCalls.Add(1)
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/v1/models", r.URL.Path)
		require.Equal(t, "Bearer new-official-fixture-key", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusUnauthorized) // 刻意拒绝，确保测试不进入保存凭据路径。
	}))
	defer current.Close()
	manifest, _ := reg.Manifest("openai")
	manifest.Auth.Defaults["base_url"] = current.URL + "/v1"
	const tenant = "f23491d2-b8cd-4107-a17b-301a44168aa6"
	require.NoError(t, db.Exec(`INSERT INTO ai_provider_credentials(env,tenant_uuid,name,provider,data) VALUES(?,?,?,?,?)`, "default", tenant, "default-openai", "openai", `{"base_url":"`+old.URL+`/v1"}`).Error)
	h := &AgentSettingHandler{svc: agentsvc.NewAgentSettingService(db)}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(reqctx.WithTenantUUID(c.Request.Context(), tenant))
	})
	router.POST("/test/connection", h.testConnection)
	request := httptest.NewRequest(http.MethodPost, "/test/connection", strings.NewReader(`{"env":"dev","modality":"llm","llm":{"provider":"openai","model":"gpt-4o","baseURL":"","apiKey":"new-official-fixture-key"}}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, 422, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "AI_CONNECTION_AUTH_FAILED")
	require.EqualValues(t, 1, defaultCalls.Load())
	require.Zero(t, oldCalls.Load())
}

func TestConnectionHTTPDoesNotInvalidateUserSessionOrSaveRejectedKey(t *testing.T) {
	previous := catalog.GlobalAIRegister
	reg := catalog.NewRegistry()
	require.NoError(t, reg.LoadByConfig(catalog.CatalogConfig{Dirs: []string{"../../../../../config/agents/providers.d"}, FailIfEmpty: true}, nil))
	catalog.GlobalAIRegister = reg
	t.Cleanup(func() { catalog.GlobalAIRegister = previous })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	defer pool.Close()
	// 刻意不创建凭据／密钥表：失败路径必须在尝试保存凭据之前返回。
	h := &AgentSettingHandler{svc: agentsvc.NewAgentSettingService(db)}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(reqctx.WithTenantUUID(c.Request.Context(), "f23491d2-b8cd-4107-a17b-301a44168aa6"))
	})
	router.POST("/test/connection", h.testConnection)
	for _, tc := range []struct {
		upstream, want int
		code           string
	}{
		{401, 422, "AI_CONNECTION_AUTH_FAILED"},
		{403, 422, "AI_CONNECTION_AUTH_FAILED"},
		{404, 501, "AI_CONNECTION_PROBE_UNSUPPORTED"},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodGet, r.Method)
			require.Equal(t, "/v1/models", r.URL.Path)
			w.WriteHeader(tc.upstream)
			_, _ = w.Write([]byte("private-provider-diagnostic"))
		}))
		body, err := json.Marshal(map[string]any{"env": "dev", "modality": "image", "image": map[string]any{"provider": "agnes", "model": "agnes-image-2.5-flash", "baseURL": srv.URL + "/v1", "apiKey": "test-rejected-key"}})
		require.NoError(t, err)
		r := httptest.NewRequest(http.MethodPost, "/test/connection", strings.NewReader(string(body)))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		srv.Close()
		require.Equal(t, tc.want, w.Code, w.Body.String())
		var response map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		require.Equal(t, tc.code, response["error_code"])
		details := response["details"].(map[string]any)
		require.EqualValues(t, tc.upstream, details["upstream_status"])
		require.Equal(t, false, details["generation_submitted"])
		require.NotContains(t, w.Body.String(), "private-provider-diagnostic")
		require.NotContains(t, w.Body.String(), "test-rejected-key")
	}
}
