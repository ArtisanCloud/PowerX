package customer

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	customersvc "github.com/ArtisanCloud/PowerX/internal/service/customer"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	audit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tenantmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/corex/tenantkeys"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestClientAppsHTTPPostgresTenantAndCredentialBoundary(t *testing.T) {
	dsn := os.Getenv("POWERX_CUSTOMER_ADMIN_TEST_DSN")
	if dsn == "" {
		t.Skip("opt-in isolated PostgreSQL HTTP acceptance")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	previous := coremodel.PowerXSchema
	schema := "client_apps_http_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, db.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	coremodel.PowerXSchema = schema
	t.Cleanup(func() {
		require.NoError(t, db.Exec(`DROP SCHEMA "`+schema+`" CASCADE`).Error)
		coremodel.PowerXSchema = previous
		require.NoError(t, pool.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.ClientApplication{}, &model.ClientAppBinding{}, &audit.AuditEvent{}, &setting.PluginInstanceConfig{}, &tenantmodel.TenantKeyPair{}))
	a, b := uuid.NewString(), uuid.NewString()
	for _, tenant := range []string{a, b} {
		require.NoError(t, db.Create(&setting.PluginInstanceConfig{TenantUUID: tenant, PluginID: "com.example.shop", Key: "auth.credentials", Enabled: true, Status: "enabled"}).Error)
	}
	wrapper, err := tenantkeys.NewDirectWrapper("http-test", "", []byte("01234567890123456789012345678901"))
	require.NoError(t, err)
	service := customersvc.NewClientApplicationService(db, tenantkeys.NewTenantKeyServiceWithWrapper(db, wrapper))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		tenant := a
		if c.GetHeader("X-Test-Tenant") == "b" {
			tenant = b
		}
		ctx := reqctx.WithUserID(reqctx.WithIsRoot(reqctx.WithTenantUUID(context.Background(), tenant), true), 42)
		if plane := c.GetHeader("X-Test-Plane"); plane != "" {
			claims := &reqctx.CoreXClaims{}
			if plane == "sts" {
				claims.Issuer = "powerx-sts"
			} else {
				claims.Platforms = []string{"api_key"}
			}
			ctx = reqctx.WithClaims(ctx, claims)
		}
		c.Request = c.Request.WithContext(ctx)
	})
	RegisterAPIRoutes(nil, router.Group(""), &shared.Deps{DB: db, CustomerClientApps: service})
	call := func(method, path string, payload any, tenant, plane string) (int, []byte) {
		raw, _ := json.Marshal(payload)
		req := httptest.NewRequest(method, "/admin/customers/client-apps"+path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-Tenant", tenant)
		req.Header.Set("X-Test-Plane", plane)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
		return response.Code, response.Body.Bytes()
	}
	secret := "dummy-http-credential"
	input := customersvc.ClientApplicationInput{Name: "HTTP application", Kind: "mini_program", Status: "active", PluginIDs: []string{"com.example.shop"}, Connections: []customersvc.ClientAppConnectionInput{{Provider: "wechat", AppID: "wx-http-test", AppSecret: &secret}}}
	for _, plane := range []string{"sts", "api_key"} {
		status, _ := call("POST", "", input, "", plane)
		require.Equal(t, 403, status)
	}
	status, body := call("POST", "", input, "", "")
	require.Equal(t, 200, status, string(body))
	require.NotContains(t, string(body), secret)
	require.NotContains(t, string(body), `"app_secret"`)
	var result struct {
		Data struct {
			Payload customersvc.ClientApplicationView `json:"payload"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &result))
	app := result.Data.Payload
	require.NotEmpty(t, app.UUID)
	status, _ = call("GET", "/"+app.UUID, nil, "b", "")
	require.Equal(t, 404, status)
	status, body = call("GET", "", nil, "b", "")
	require.Equal(t, 200, status)
	require.Contains(t, string(body), `"total":0`)
	status, body = call("POST", "", input, "b", "")
	require.Equal(t, 200, status, string(body), "same AppID is independently registered in tenant B")
	input.ExpectedVersion = app.Version
	input.Connections[0].UUID = app.Connections[0].UUID
	input.Connections[0].AppSecret = nil
	input.Status = "disabled"
	status, body = call("PATCH", "/"+app.UUID, input, "", "")
	require.Equal(t, 200, status, string(body))
	status, body = call("PATCH", "/"+app.UUID, input, "", "")
	require.Equal(t, 409, status)
	require.Contains(t, string(body), "CLIENT_APP_VERSION_CONFLICT")
	_, _, err = service.Lookup(context.Background(), a, "com.example.shop", "wechat_miniapp", "wx-http-test")
	require.ErrorIs(t, err, customersvc.ErrCustomerClientAppForbidden)
}
