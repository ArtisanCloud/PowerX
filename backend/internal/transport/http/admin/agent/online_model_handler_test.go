package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	agentsvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestOnlineModelSyncHTTPUsesAdminGuardStrictBodyAndPersistsAuthoritatively(t *testing.T) {
	gin.SetMode(gin.TestMode)
	old := catalog.GlobalAIRegister
	oldSchema := core.PowerXSchema
	core.PowerXSchema = "main"
	reg := catalog.NewRegistry()
	require.NoError(t, reg.LoadByConfig(catalog.CatalogConfig{Dirs: []string{"../../../../../config/agents/providers.d"}, FailIfEmpty: true}, nil))
	catalog.GlobalAIRegister = reg
	t.Cleanup(func() { catalog.GlobalAIRegister = old; core.PowerXSchema = oldSchema })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	for _, statement := range []string{
		`CREATE TABLE ai_provider_credentials(id integer primary key, created_at datetime, updated_at datetime, deleted_at datetime, env text, tenant_uuid text, name text, provider text, auth_scheme text, data text)`,
		`CREATE TABLE cfg_tenant_settings(id integer primary key, created_at datetime, updated_at datetime, deleted_at datetime, tenant_uuid text, key text, value_json text, "group" text, description text, editable boolean, UNIQUE(tenant_uuid,key))`,
	} {
		require.NoError(t, db.Exec(statement).Error)
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			require.Equal(t, http.MethodGet, r.Method)
			_, _ = w.Write([]byte(`{"models":[{"name":"custom:14b"}]}`))
		} else {
			require.Equal(t, "/api/show", r.URL.Path)
			_, _ = w.Write([]byte(`{"capabilities":["completion"]}`))
		}
	}))
	defer provider.Close()
	const tenant = "11111111-1111-4111-8111-111111111111"
	require.NoError(t, db.Exec(`INSERT INTO ai_provider_credentials(env,tenant_uuid,name,provider,data) VALUES(?,?,?,?,?)`, "default", tenant, "default-ollama", "ollama", `{"base_url":"`+provider.URL+`"}`).Error)
	h := &AgentSettingHandler{svc: agentsvc.NewAgentSettingService(db)}
	router := gin.New()
	router.POST("/models/sync", requireOnlineModelCatalogAdmin(nil), h.syncOnlineModels)
	router.GET("/credentials", h.listCredentials)
	router.GET("/providers", h.listProviders)
	call := func(ctx context.Context, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/models/sync", strings.NewReader(body)).WithContext(ctx)
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		return w
	}
	base := reqctx.WithTenantUUID(context.Background(), tenant)
	admin := reqctx.WithIsRoot(base, true)
	body := `{"env":"dev","provider":"ollama","modality":"llm","action":"refresh"}`
	denied := call(base, body)
	require.Equal(t, 403, denied.Code)
	require.Contains(t, denied.Body.String(), "AI_MODEL_SYNC_ADMIN_REQUIRED")
	apiContext := reqctx.WithClaims(admin, &reqctx.CoreXClaims{Platforms: []string{"api_key"}, TenantUUID: tenant})
	require.Equal(t, 403, call(apiContext, body).Code, "service identity cannot inherit a user/root flag")
	stsClaims := &reqctx.CoreXClaims{TenantUUID: tenant}
	stsClaims.Issuer = "powerx-sts"
	require.Equal(t, 403, call(reqctx.WithClaims(admin, stsClaims), body).Code)
	for _, invalid := range []string{`{"env":"dev","provider":"ollama","action":"refresh","endpoint":"http://evil.invalid"}`, body + ` {}`, strings.Repeat("x", 66000)} {
		w := call(admin, invalid)
		require.Equal(t, 400, w.Code)
	}
	missingConfirmation := call(admin, `{"env":"dev","provider":"openai","modality":"llm","action":"assign","revision":"fixture","models":["unknown"]}`)
	require.Equal(t, 422, missingConfirmation.Code)
	require.Contains(t, missingConfirmation.Body.String(), "AI_MODEL_SYNC_CONFIRMATION_REQUIRED")
	bulk := call(admin, `{"env":"dev","provider":"openai","modality":"llm","action":"assign","revision":"fixture","models":["unknown-a","unknown-b"],"confirm_unknown":true}`)
	require.Equal(t, 422, bulk.Code)
	require.Contains(t, bulk.Body.String(), "AI_MODEL_SYNC_ASSIGNMENT_INVALID")
	w := call(admin, body)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"id":"custom:14b"`)
	require.Contains(t, w.Body.String(), `"match":"provider"`)
	require.NotContains(t, w.Body.String(), "connection_fingerprint")
	var count int64
	require.NoError(t, db.Table("cfg_tenant_settings").Count(&count).Error)
	require.EqualValues(t, 1, count)
	for _, env := range []string{"default", "dev"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/credentials?env="+env, nil).WithContext(admin))
		require.Equal(t, 200, response.Code, response.Body.String())
		require.Contains(t, response.Body.String(), `"env":"dev"`)
		require.Contains(t, response.Body.String(), `"name":"default-ollama"`)
		response = httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/providers?env="+env+"&modality=llm", nil).WithContext(admin))
		require.Equal(t, 200, response.Code, response.Body.String())
		var out struct {
			Data struct {
				Providers []providerView `json:"providers"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &out))
		found := false
		for _, p := range out.Data.Providers {
			if p.ID == "ollama" {
				found = true
				require.True(t, p.Configured)
			}
		}
		require.True(t, found)
	}
	// Exercise the actual error bridge. AppError details used to be lost here,
	// leaving the browser without an upstream status, endpoint or safe reason.
	deniedProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"老版本免费API已失效，请绑定GitHub。fixture-secret"}}`))
	}))
	defer deniedProvider.Close()
	require.NoError(t, db.Exec(`INSERT INTO ai_provider_credentials(env,tenant_uuid,name,provider,data) VALUES(?,?,?,?,?)`, "dev", tenant, "dev-openai-compatible", "openai_compatible", `{"base_url":"`+deniedProvider.URL+`","api_key":"fixture-secret"}`).Error)
	w = call(admin, `{"env":"dev","provider":"openai_compatible","modality":"llm","action":"refresh"}`)
	require.Equal(t, 422, w.Code, w.Body.String())
	var failure struct {
		ErrorCode string         `json:"error_code"`
		Details   map[string]any `json:"details"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &failure))
	require.Equal(t, "AI_MODEL_SYNC_AUTH_FAILED", failure.ErrorCode)
	require.Equal(t, float64(403), failure.Details["upstream_status"])
	require.Equal(t, deniedProvider.URL+"/v1/models", failure.Details["source"])
	require.Equal(t, "legacy_free_key_retired", failure.Details["upstream_reason"])
	require.NotContains(t, w.Body.String(), "fixture-secret")
	require.NoError(t, db.Table("cfg_tenant_settings").Count(&count).Error)
	require.EqualValues(t, 1, count, "a denied update cannot modify any existing snapshots")
}
