package integration_gateway

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"testing"

	owners "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
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

func TestProfileHTTPPluginGrantSaveCreateRotateRevokeAndIsolation(t *testing.T) {
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
	handler := NewAPIKeyAdminHandler(db)
	handler.templatesEnsuredOnce = true
	router := gin.New()
	router.Use(func(c *gin.Context) {
		current := tenant
		if c.GetHeader("X-Test-Tenant") != "" {
			current = c.GetHeader("X-Test-Tenant")
		}
		claims := &reqctx.CoreXClaims{UserID: 1, IsRoot: true, TenantUUID: current}
		ctx := reqctx.WithTenantUUID(reqctx.WithClaims(c.Request.Context(), claims), current)
		if c.GetHeader("X-Test-Auth") == "api_key" {
			ctx = reqctx.WithAuthenticatedAPIKeyHash(ctx, "fixture")
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/admin/integration/api-key-profiles/:profile_id/permissions", handler.SetAPIKeyProfilePermissions)
	router.POST("/admin/integration/api-key-profiles/:profile_id/permissions/append", handler.AppendAPIKeyProfilePermissions)
	router.POST("/admin/integration/api-keys", handler.CreateAPIKey)
	router.POST("/admin/integration/api-keys/:key_id/rotate", handler.RotateAPIKey)
	call := func(method, path string, body any, auth, foreign string, status int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		request := httptest.NewRequest(method, path, bytes.NewReader(raw))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Test-Auth", auth)
		request.Header.Set("X-Test-Tenant", foreign)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.Equal(t, status, response.Code, response.Body.String())
		var result map[string]any
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
		return result
	}
	require.NoError(t, db.AutoMigrate(&iammodel.APIKey{}))
	profilePath := "/admin/integration/api-key-profiles/" + strconv.FormatUint(profile.ID, 10) + "/permissions"
	body := map[string]any{"permission_ids": []uint64{permission.ID}}
	for _, method := range []string{"PUT", "POST"} {
		path := profilePath
		if method == "POST" {
			path += "/append"
		}
		call(method, path, body, "api_key", "", 403)
		call(method, path, body, "", uuid.NewString(), 404)
		call(method, path, body, "", "", 200)
	}
	var rows []gwmodel.IntegrationGatewayAPIKeyPermission
	require.NoError(t, db.Where("api_key_uuid = ?", key.UUID).Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, provider, rows[0].PluginID)
	var stored gwmodel.IntegrationGatewayAPIKey
	require.NoError(t, db.First(&stored, "uuid = ?", key.UUID).Error)
	policy, err := owners.ResolveOwnerPolicy(stored.PluginOwnerPolicy, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"com.powerx.plugins.workspacex"}, policy.Owners())
	require.Empty(t, policy.ExplicitGrants)
	// A Key created from this Profile inherits provider read without becoming an Ecommerce resource owner.
	created := call("POST", "/admin/integration/api-keys", map[string]any{"profile_id": profile.ID, "name": "new-consumer"}, "", "", 201)["data"].(map[string]any)["api_key"].(map[string]any)
	createdID := created["key_id"].(string)
	require.Empty(t, created["plugin_ids"])
	require.NoError(t, db.Where("api_key_uuid = ?", createdID).Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, provider, rows[0].PluginID)
	rotated := call("POST", "/admin/integration/api-keys/"+key.UUID.String()+"/rotate", map[string]any{}, "", "", 200)["data"].(map[string]any)["api_key"].(map[string]any)
	rotatedID := rotated["key_id"].(string)
	require.Equal(t, []any{"com.powerx.plugins.workspacex"}, rotated["plugin_ids"])
	require.NoError(t, db.Where("api_key_uuid = ?", rotatedID).Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, provider, rows[0].PluginID)
	call("PUT", profilePath, map[string]any{"permission_ids": []uint64{}}, "", "", 200)
	for _, id := range []string{createdID, rotatedID} {
		var n int64
		require.NoError(t, db.Model(&gwmodel.IntegrationGatewayAPIKeyPermission{}).Where("api_key_uuid = ?", id).Count(&n).Error)
		require.Zero(t, n)
	}
	require.NoError(t, db.Model(&capmodels.CapabilityRegistration{}).Where("tenant_uuid = ?", tenant).Update("status", "disabled").Error)
	failed := call("PUT", profilePath, body, "", "", 409)
	require.Equal(t, "CAPABILITY_TENANT_NOT_REGISTERED", failed["error_code"])
	var count int64
	require.NoError(t, db.Model(&iammodel.APIKeyProfilePermission{}).Where("profile_id = ?", profile.ID).Count(&count).Error)
	require.Zero(t, count)
}
