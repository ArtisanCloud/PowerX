package integration_gateway

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	owners "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/manager"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	iammodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gwmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/pluginpermission"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestBatchGrantRegisteredHTTPRouteAndStrictAuthorization(t *testing.T) {
	old := model.PowerXSchema
	model.PowerXSchema = "main"
	t.Cleanup(func() { model.PowerXSchema = old })
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&iammodel.Permission{}, &iammodel.APIKeyProfile{}, &iammodel.APIKeyProfilePermission{}, &gwmodel.IntegrationGatewayAPIKey{}, &gwmodel.IntegrationGatewayAPIKeyPermission{}, &gwmodel.IntegrationGatewayAPIKeyAuditLog{}, &capmodels.CapabilityRecord{}, &capmodels.CapabilityRegistration{}))
	tenant := uuid.NewString()
	provider := "com.powerx.plugins.ecommerce"
	capability := provider + ".product.spu.subscription.plan.read"
	meta, err := pluginpermission.ExplicitAPIKeyMetadata(&pluginpermission.APIKeySpec{CapabilityID: capability, PluginID: provider, Scope: "_scope.ecommerce.subscription_plans.read", Action: "read", ResourceType: "capability", ResourcePattern: "subscription_plans", Effect: "allow"}, provider, capability, true)
	require.NoError(t, err)
	raw, err := json.Marshal(meta)
	require.NoError(t, err)
	permission := iammodel.Permission{Module: "commerce", Resource: "plans_service", Action: "read", Source: "plugin:" + provider, Status: iammodel.PermissionStatusActive, AllowAPIKey: true, Meta: datatypes.JSON(raw)}
	require.NoError(t, db.Create(&permission).Error)
	require.NoError(t, db.Create(&capmodels.CapabilityRecord{CapabilityID: capability, PluginID: provider, PluginVersion: "0.3.1", Status: "published", Title: "Plans"}).Error)
	require.NoError(t, db.Create(&capmodels.CapabilityRegistration{CapabilityID: capability, TenantUUID: tenant, Status: "published", Version: 1}).Error)
	profile := iammodel.APIKeyProfile{TenantUUID: tenant, Key: "http-batch", Name: "HTTP batch", Status: 1}
	require.NoError(t, db.Create(&profile).Error)
	key := gwmodel.IntegrationGatewayAPIKey{TenantUUID: tenant, ProfileID: profile.ID, Name: "WorkspaceX", KeyHash: "never-return-fixture-hash", Status: "active", PluginOwnerPolicy: owners.EncodePluginOwners([]string{"com.powerx.plugins.workspacex"})}
	require.NoError(t, db.Create(&key).Error)
	router := gin.New()
	protected := router.Group("/custom/v8")
	protected.Use(func(c *gin.Context) {
		current := tenant
		if c.GetHeader("X-Test-Tenant") != "" {
			current = c.GetHeader("X-Test-Tenant")
		}
		ctx := reqctx.WithTenantUUID(reqctx.WithClaims(c.Request.Context(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true, TenantUUID: current}), current)
		switch c.GetHeader("X-Test-Auth") {
		case "api_key":
			ctx = reqctx.WithAuthenticatedAPIKeyHash(ctx, "fixture")
		case "sts":
			claims := &reqctx.CoreXClaims{TenantUUID: current}
			claims.Issuer = "powerx-sts"
			ctx = reqctx.WithClaims(ctx, claims)
		case "member":
			ctx = reqctx.WithClaims(ctx, &reqctx.CoreXClaims{UserID: 9, TenantUUID: current, Roles: []string{"role_user"}})
		}
		c.Request = c.Request.WithContext(reqctx.WithTraceID(ctx, "batch-http-trace"))
		c.Next()
	})
	RegisterAPIRoutes(nil, protected, &shared.Deps{DB: db, APIKeyOwners: owners.NewAPIKeyOwnerService(db), IntegrationGateway: &shared.IntegrationGatewayDeps{Manager: &manager.Service{}}})
	call := func(body string, auth, foreign string, status int) map[string]any {
		t.Helper()
		request := httptest.NewRequest("POST", "/custom/v8/admin/integration/api-key-grants/batch", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Test-Auth", auth)
		request.Header.Set("X-Test-Tenant", foreign)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.Equal(t, status, response.Code, response.Body.String())
		require.NotContains(t, response.Body.String(), key.KeyHash)
		var out map[string]any
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &out))
		return out
	}
	input := map[string]any{"key_ids": []string{key.UUID.String()}, "permission_ids": []uint64{permission.ID}, "dry_run": true}
	encode := func() string { raw, _ := json.Marshal(input); return string(raw) }
	for _, auth := range []string{"api_key", "sts", "member"} {
		response := call(encode(), auth, "", 403)
		require.Equal(t, "API_KEY_OWNER_ADMIN_REQUIRED", response["error_code"])
	}
	call(encode(), "", uuid.NewString(), 404)
	for _, bad := range []string{`{"tenant_uuid":"ignored"}`, `{"key_ids":["not-uuid"]}`, encode() + ` {}`, encode() + strings.Repeat(" ", 65536), `{"key_ids":null}`, `{"scope":"*"}`} {
		call(bad, "", "", 400)
	}
	preview := call(encode(), "", "", 200)["data"].(map[string]any)
	require.Equal(t, false, preview["committed"])
	var count int64
	require.NoError(t, db.Model(&gwmodel.IntegrationGatewayAPIKeyAuditLog{}).Count(&count).Error)
	require.Zero(t, count)
	input["dry_run"] = false
	call(encode(), "", "", 400)
	input["expected_plan_hash"] = "stale"
	stale := call(encode(), "", "", 409)
	require.Equal(t, "API_KEY_BATCH_PREVIEW_STALE", stale["error_code"])
	require.Equal(t, "batch-http-trace", stale["details"].(map[string]any)["trace_id"])
	input["expected_plan_hash"] = preview["plan_hash"]
	result := call(encode(), "", "", 200)["data"].(map[string]any)
	require.Equal(t, true, result["committed"])
	require.NoError(t, db.Model(&gwmodel.IntegrationGatewayAPIKeyAuditLog{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, db.Model(&capmodels.CapabilityRegistration{}).Where("tenant_uuid = ?", tenant).Update("status", "disabled").Error)
	input["dry_run"] = true
	failed := call(encode(), "", "", 409)
	require.Equal(t, "CAPABILITY_TENANT_NOT_REGISTERED", failed["error_code"])
	require.EqualValues(t, permission.ID, failed["details"].(map[string]any)["failed_permission_id"])
}
